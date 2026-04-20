# ltts

Interactive TUI that converts a text file (`.txt`, `.md`, `.html`) into an MP3
using a text-to-speech engine of your choice:

- **Google Translate TTS** — online, no install, decent quality.
- **Piper** — local, ~25 MB binary + ~60 MB voice, fast and offline.
- **Kokoro-82M** — local, ~340 MB model, most natural voice of the three.

On first use of Piper or Kokoro, ltts downloads and sets everything up under
`~/.cache/ltts/`.

## Build

```
go build ./...
```

Requires:

- `ffmpeg` on `$PATH` (used for concatenation / WAV→MP3 encoding).
- `python3` (with `venv`) on `$PATH` only if you pick the Kokoro engine.

## Run

```
./ltts
```

The TUI walks you through:

1. Pick an input file (tab-completion via your terminal).
2. Choose an engine. If it is not installed yet, ltts will offer to install it
   — Piper downloads a single static binary + one voice; Kokoro creates a
   Python venv under `~/.cache/ltts/kokoro/venv` and pulls the ONNX model.
3. Pick a voice / language.
4. Confirm the output MP3 path.
5. Watch the chunk-by-chunk progress.

## Engine notes

- **Google** splits input into ~180-char chunks against the undocumented
  `translate_tts` endpoint. It can rate-limit.
- **Piper** sends the full text to its CLI (no chunking needed) and produces
  WAV that ltts then encodes to MP3. Voices live under
  `~/.cache/ltts/piper/voices/`.
- **Kokoro** runs the `kokoro-onnx` package inside a dedicated venv at
  `~/.cache/ltts/kokoro/venv`. Chunks are capped at ~500 chars.

To reset an engine, delete its folder under `~/.cache/ltts/` and pick it again.
