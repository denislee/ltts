package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Voice is an engine-specific voice selection.
type Voice struct {
	ID    string // engine-internal identifier (e.g. "en_US-amy-medium", "af_bella", "en")
	Label string // display label shown in the TUI
	Lang  string // BCP-47-ish code, "en", "es", etc.
}

// Engine is a text-to-speech backend.
type Engine interface {
	ID() string
	Name() string
	Description() string
	// Ready reports whether Install has been run successfully (or is unneeded).
	Ready() bool
	// Install downloads/sets up whatever the engine needs. Safe to call when Ready.
	// log receives human-readable progress lines.
	Install(ctx context.Context, log func(string)) error
	// Voices returns the available voices once Ready. Should be fast.
	Voices() []Voice
	// ChunkLimit is the max input size per Synth call, in runes. 0 = no limit.
	ChunkLimit() int
	// Synth writes a single chunk to dst. Format must match Format().
	Synth(ctx context.Context, text string, voice Voice, slow bool, dst string) error
	// Format is the audio format Synth produces, either "mp3" or "wav".
	Format() string
}

// Engines returns all registered engines in display order.
func Engines() []Engine {
	return []Engine{
		&googleEngine{},
		&piperEngine{},
		&kokoroEngine{},
	}
}

// cacheDir returns ~/.cache/ltts (created on demand).
func cacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	d := filepath.Join(base, "ltts")
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// ensureFFmpeg errors if ffmpeg is not on PATH.
func ensureFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("ffmpeg is required on PATH to produce MP3: %w", err)
	}
	return nil
}

// Synthesize runs the engine over text, chunking as needed, and stitches the
// result into outMP3. Progress is reported via onProgress(i, total).
func Synthesize(
	ctx context.Context,
	eng Engine,
	text string,
	voice Voice,
	slow bool,
	outMP3 string,
	onProgress func(i, total int),
) error {
	if err := ensureFFmpeg(); err != nil {
		return err
	}

	var chunks []string
	if lim := eng.ChunkLimit(); lim > 0 {
		chunks = splitText(text, lim)
	} else {
		chunks = []string{text}
	}
	if len(chunks) == 0 {
		return fmt.Errorf("nothing to synthesize")
	}

	tmp, err := os.MkdirTemp("", "ltts-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	ext := eng.Format()
	if ext != "mp3" && ext != "wav" {
		return fmt.Errorf("engine %s: unsupported format %q", eng.ID(), ext)
	}

	listFile := filepath.Join(tmp, "list.txt")
	lf, err := os.Create(listFile)
	if err != nil {
		return err
	}

	for i, c := range chunks {
		if onProgress != nil {
			onProgress(i, len(chunks))
		}
		part := filepath.Join(tmp, fmt.Sprintf("part-%04d.%s", i, ext))
		if err := eng.Synth(ctx, c, voice, slow, part); err != nil {
			lf.Close()
			return fmt.Errorf("chunk %d/%d: %w", i+1, len(chunks), err)
		}
		fmt.Fprintf(lf, "file '%s'\n", part)
	}
	if onProgress != nil {
		onProgress(len(chunks), len(chunks))
	}
	if err := lf.Close(); err != nil {
		return err
	}

	// Encode to MP3. For MP3 inputs we stream-copy; for WAV we re-encode.
	args := []string{"-y", "-loglevel", "error",
		"-f", "concat", "-safe", "0", "-i", listFile}
	if ext == "mp3" {
		args = append(args, "-c", "copy")
	} else {
		args = append(args, "-c:a", "libmp3lame", "-q:a", "4")
	}
	args = append(args, outMP3)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
