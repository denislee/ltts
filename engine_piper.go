package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// piperEngine wraps the piper CLI (rhasspy/piper). It auto-downloads the
// binary and a curated set of voice models on first use.
type piperEngine struct{}

func (p *piperEngine) ID() string   { return "piper" }
func (p *piperEngine) Name() string { return "Piper (local, fast)" }
func (p *piperEngine) Description() string {
	return "Lightweight offline neural TTS by rhasspy. Downloads ~25 MB binary and ~60 MB voice."
}

const piperReleaseTag = "2023.11.14-2"

// piperVoice is a curated list of good-quality voices across a few languages.
// See https://huggingface.co/rhasspy/piper-voices for the full catalog.
type piperVoice struct {
	ID     string // e.g. en_US-amy-medium
	Label  string
	Lang   string
	Locale string // e.g. en/en_US
	Name   string // speaker name, e.g. amy
	Qual   string // low/medium/high
}

func piperVoiceCatalog() []piperVoice {
	return []piperVoice{
		{"en_US-amy-medium",       "English (US) — Amy",     "en", "en/en_US", "amy",       "medium"},
		{"en_US-lessac-medium",    "English (US) — Lessac",  "en", "en/en_US", "lessac",    "medium"},
		{"en_US-ryan-high",        "English (US) — Ryan",    "en", "en/en_US", "ryan",      "high"},
		{"en_GB-alan-medium",      "English (UK) — Alan",    "en", "en/en_GB", "alan",      "medium"},
		{"es_ES-sharvard-medium",  "Spanish (ES) — Sharvard", "es", "es/es_ES", "sharvard", "medium"},
		{"es_MX-claude-high",      "Spanish (MX) — Claude",  "es", "es/es_MX", "claude",    "high"},
		{"pt_BR-faber-medium",     "Portuguese (BR) — Faber", "pt", "pt/pt_BR", "faber",    "medium"},
		{"fr_FR-siwis-medium",     "French — Siwis",         "fr", "fr/fr_FR", "siwis",     "medium"},
		{"de_DE-thorsten-medium",  "German — Thorsten",      "de", "de/de_DE", "thorsten",  "medium"},
		{"it_IT-riccardo-x_low",   "Italian — Riccardo",     "it", "it/it_IT", "riccardo",  "x_low"},
	}
}

func (p *piperEngine) piperDir() (string, error) {
	c, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(c, "piper"), nil
}

func (p *piperEngine) binaryPath() string {
	dir, _ := p.piperDir()
	return filepath.Join(dir, "piper")
}

func (p *piperEngine) voicesDir() string {
	dir, _ := p.piperDir()
	return filepath.Join(dir, "voices")
}

func (p *piperEngine) voicePath(v piperVoice) (onnx, cfg string) {
	d := p.voicesDir()
	return filepath.Join(d, v.ID+".onnx"), filepath.Join(d, v.ID+".onnx.json")
}

func (p *piperEngine) Ready() bool {
	if !fileExists(p.binaryPath()) {
		return false
	}
	for _, v := range piperVoiceCatalog() {
		onnx, cfg := p.voicePath(v)
		if fileExists(onnx) && fileExists(cfg) {
			return true
		}
	}
	return false
}

func (p *piperEngine) Install(ctx context.Context, log func(string)) error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("automatic piper install currently only supports linux; got %s", runtime.GOOS)
	}
	if err := p.installBinary(ctx, log); err != nil {
		return err
	}
	// Install one default voice so the engine is usable immediately. Users can
	// pick from the full catalog on the voice screen — we download on demand
	// via ensureVoice.
	def := piperVoiceCatalog()[0]
	if err := p.ensureVoice(ctx, def, log); err != nil {
		return err
	}
	return nil
}

func (p *piperEngine) installBinary(ctx context.Context, log func(string)) error {
	if fileExists(p.binaryPath()) {
		return nil
	}
	dir, err := p.piperDir()
	if err != nil {
		return err
	}
	arch := runtime.GOARCH
	var assetArch string
	switch arch {
	case "amd64":
		assetArch = "x86_64"
	case "arm64":
		assetArch = "aarch64"
	case "arm":
		assetArch = "armv7"
	default:
		return fmt.Errorf("unsupported arch %s for piper", arch)
	}
	url := fmt.Sprintf("https://github.com/rhasspy/piper/releases/download/%s/piper_linux_%s.tar.gz", piperReleaseTag, assetArch)
	tmp := filepath.Join(dir, "piper.tar.gz")
	if err := downloadFile(ctx, url, tmp, log); err != nil {
		return err
	}
	defer os.Remove(tmp)
	if log != nil {
		log("extracting piper binary")
	}
	if err := extractTarGz(tmp, dir, true); err != nil {
		return err
	}
	// Make sure the binary is executable.
	_ = os.Chmod(p.binaryPath(), 0o755)
	return nil
}

func (p *piperEngine) ensureVoice(ctx context.Context, v piperVoice, log func(string)) error {
	onnx, cfg := p.voicePath(v)
	if fileExists(onnx) && fileExists(cfg) {
		return nil
	}
	base := fmt.Sprintf("https://huggingface.co/rhasspy/piper-voices/resolve/main/%s/%s/%s", v.Locale, v.Name, v.Qual)
	if err := downloadFile(ctx, base+"/"+v.ID+".onnx", onnx, log); err != nil {
		return fmt.Errorf("download voice onnx: %w", err)
	}
	if err := downloadFile(ctx, base+"/"+v.ID+".onnx.json", cfg, log); err != nil {
		return fmt.Errorf("download voice cfg: %w", err)
	}
	return nil
}

func (p *piperEngine) Voices() []Voice {
	out := make([]Voice, 0, len(piperVoiceCatalog()))
	for _, v := range piperVoiceCatalog() {
		out = append(out, Voice{ID: v.ID, Label: v.Label, Lang: v.Lang})
	}
	return out
}

func (p *piperEngine) ChunkLimit() int { return 0 }
func (p *piperEngine) Format() string  { return "wav" }

func (p *piperEngine) lookupVoice(id string) (piperVoice, bool) {
	for _, v := range piperVoiceCatalog() {
		if v.ID == id {
			return v, true
		}
	}
	return piperVoice{}, false
}

func (p *piperEngine) Synth(ctx context.Context, text string, voice Voice, slow bool, dst string) error {
	pv, ok := p.lookupVoice(voice.ID)
	if !ok {
		return fmt.Errorf("piper: unknown voice %q", voice.ID)
	}
	if err := p.ensureVoice(ctx, pv, nil); err != nil {
		return err
	}
	onnx, _ := p.voicePath(pv)

	args := []string{"--model", onnx, "--output_file", dst}
	if slow {
		args = append(args, "--length_scale", "1.35")
	}
	cmd := exec.CommandContext(ctx, p.binaryPath(), args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := io.WriteString(stdin, text); err != nil {
		stdin.Close()
		_ = cmd.Wait()
		return err
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("piper: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
