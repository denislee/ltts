package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// kokoroEngine runs Kokoro-82M via the kokoro-onnx python package in a
// dedicated venv at ~/.cache/ltts/kokoro. First use downloads ~340 MB.
type kokoroEngine struct{}

func (k *kokoroEngine) ID() string   { return "kokoro" }
func (k *kokoroEngine) Name() string { return "Kokoro-82M (local, most natural)" }
func (k *kokoroEngine) Description() string {
	return "82M-param neural TTS. Most natural voice of the three. Downloads ~340 MB and needs python3 + pip."
}

const (
	kokoroModelURL  = "https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/kokoro-v1.0.onnx"
	kokoroVoicesURL = "https://github.com/thewh1teagle/kokoro-onnx/releases/download/model-files-v1.0/voices-v1.0.bin"
)

// kokoroVoiceCatalog is a curated subset of Kokoro voices. The first letter of
// the ID encodes region/gender: a=American, b=British; f=female, m=male.
func kokoroVoiceCatalog() []Voice {
	return []Voice{
		{ID: "af_bella",    Label: "English (US F) — Bella",    Lang: "en"},
		{ID: "af_sarah",    Label: "English (US F) — Sarah",    Lang: "en"},
		{ID: "af_nicole",   Label: "English (US F) — Nicole",   Lang: "en"},
		{ID: "af_sky",      Label: "English (US F) — Sky",      Lang: "en"},
		{ID: "am_adam",     Label: "English (US M) — Adam",     Lang: "en"},
		{ID: "am_michael",  Label: "English (US M) — Michael",  Lang: "en"},
		{ID: "bf_emma",     Label: "English (UK F) — Emma",     Lang: "en"},
		{ID: "bf_isabella", Label: "English (UK F) — Isabella", Lang: "en"},
		{ID: "bm_george",   Label: "English (UK M) — George",   Lang: "en"},
		{ID: "bm_lewis",    Label: "English (UK M) — Lewis",    Lang: "en"},
	}
}

func (k *kokoroEngine) baseDir() (string, error) {
	c, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(c, "kokoro"), nil
}

func (k *kokoroEngine) venvDir() string    { d, _ := k.baseDir(); return filepath.Join(d, "venv") }
func (k *kokoroEngine) pythonBin() string  { return filepath.Join(k.venvDir(), "bin", "python") }
func (k *kokoroEngine) pipBin() string     { return filepath.Join(k.venvDir(), "bin", "pip") }
func (k *kokoroEngine) modelPath() string  { d, _ := k.baseDir(); return filepath.Join(d, "kokoro-v1.0.onnx") }
func (k *kokoroEngine) voicesPath() string { d, _ := k.baseDir(); return filepath.Join(d, "voices-v1.0.bin") }
func (k *kokoroEngine) workerPath() string { d, _ := k.baseDir(); return filepath.Join(d, "synth.py") }

func (k *kokoroEngine) Ready() bool {
	return fileExists(k.pythonBin()) &&
		fileExists(k.modelPath()) &&
		fileExists(k.voicesPath()) &&
		fileExists(k.workerPath())
}

func (k *kokoroEngine) Install(ctx context.Context, log func(string)) error {
	if _, err := exec.LookPath("python3"); err != nil {
		return fmt.Errorf("python3 not found on PATH; install python3 (with venv) first")
	}
	base, err := k.baseDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}

	if !fileExists(k.pythonBin()) {
		if log != nil {
			log("creating python venv")
		}
		cmd := exec.CommandContext(ctx, "python3", "-m", "venv", k.venvDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("python -m venv failed: %w\n%s", err, out)
		}
	}

	if log != nil {
		log("installing kokoro-onnx (may take a minute)")
	}
	cmd := exec.CommandContext(ctx, k.pipBin(), "install", "--quiet",
		"kokoro-onnx", "soundfile", "numpy")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pip install failed: %w", err)
	}

	if !fileExists(k.modelPath()) {
		if err := downloadFile(ctx, kokoroModelURL, k.modelPath(), log); err != nil {
			return err
		}
	}
	if !fileExists(k.voicesPath()) {
		if err := downloadFile(ctx, kokoroVoicesURL, k.voicesPath(), log); err != nil {
			return err
		}
	}

	if err := os.WriteFile(k.workerPath(), []byte(kokoroWorker), 0o755); err != nil {
		return err
	}
	return nil
}

func (k *kokoroEngine) Voices() []Voice { return kokoroVoiceCatalog() }
func (k *kokoroEngine) ChunkLimit() int { return 500 }
func (k *kokoroEngine) Format() string  { return "wav" }

func (k *kokoroEngine) Synth(ctx context.Context, text string, voice Voice, slow bool, dst string) error {
	speed := 1.0
	if slow {
		speed = 0.75
	}
	payload := map[string]any{
		"model":  k.modelPath(),
		"voices": k.voicesPath(),
		"voice":  voice.ID,
		"text":   text,
		"lang":   "en-us",
		"speed":  speed,
		"out":    dst,
	}
	buf, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, k.pythonBin(), k.workerPath())
	cmd.Stdin = strings.NewReader(string(buf))
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("kokoro: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

const kokoroWorker = `#!/usr/bin/env python3
import json, sys, soundfile as sf
from kokoro_onnx import Kokoro

req = json.load(sys.stdin)
k = Kokoro(req["model"], req["voices"])
samples, rate = k.create(
    req["text"], voice=req["voice"], speed=float(req["speed"]),
    lang=req.get("lang", "en-us"),
)
sf.write(req["out"], samples, rate)
`
