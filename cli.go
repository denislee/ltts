package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cliOpts holds everything runCLI needs. Populated from flags in main.
type cliOpts struct {
	input        string
	output       string
	engineID     string
	voiceID      string
	slow         bool
	yes          bool
	listEngines  bool
	listVoices   bool
}

// parseFlags wires the CLI surface. Returns opts and whether any CLI-mode
// flag was provided (if not, caller should fall back to the TUI).
func parseFlags(args []string) (cliOpts, bool, error) {
	fs := flag.NewFlagSet("ltts", flag.ContinueOnError)
	var o cliOpts
	fs.StringVar(&o.input, "input", "", "input file (.txt/.md/.html) — enables CLI mode")
	fs.StringVar(&o.output, "output", "", "output MP3 path (default: <input>.mp3)")
	fs.StringVar(&o.engineID, "engine", "google", "TTS engine: google, piper, kokoro")
	fs.StringVar(&o.voiceID, "voice", "", "voice ID (default: engine's first voice)")
	fs.BoolVar(&o.slow, "slow", false, "slower speech")
	fs.BoolVar(&o.yes, "yes", false, "auto-install the engine if missing")
	fs.BoolVar(&o.listEngines, "list-engines", false, "print available engines and exit")
	fs.BoolVar(&o.listVoices, "list-voices", false, "print voices for -engine and exit")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(),
			"ltts — text-to-speech to MP3\n\n"+
				"Usage:\n"+
				"  ltts                              # interactive TUI\n"+
				"  ltts -input FILE [flags]          # CLI mode\n"+
				"  ltts -list-engines\n"+
				"  ltts -engine piper -list-voices\n\n"+
				"Flags:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return o, false, err
	}
	cliMode := o.input != "" || o.listEngines || o.listVoices
	return o, cliMode, nil
}

// runCLI executes the non-interactive path. Progress lines go to stderr so
// stdout is free for structured output like -list-voices.
func runCLI(o cliOpts) error {
	engines := Engines()

	if o.listEngines {
		for _, e := range engines {
			mark := ""
			if e.Ready() {
				mark = " (installed)"
			}
			fmt.Printf("%s%s\t%s\n", e.ID(), mark, e.Description())
		}
		return nil
	}

	eng := pickEngine(engines, o.engineID)
	if eng == nil {
		return fmt.Errorf("unknown engine %q (try -list-engines)", o.engineID)
	}

	if o.listVoices {
		// Some engines only populate Voices() after install (e.g. piper).
		if !eng.Ready() {
			fmt.Fprintf(os.Stderr, "note: %s is not installed; voice list may be empty\n", eng.Name())
		}
		for _, v := range eng.Voices() {
			fmt.Printf("%s\t%s\t%s\n", v.ID, v.Lang, v.Label)
		}
		return nil
	}

	if o.input == "" {
		return fmt.Errorf("-input is required in CLI mode")
	}

	// Install if needed.
	if !eng.Ready() {
		if !o.yes {
			return fmt.Errorf("%s is not installed; pass -yes to auto-install, or run ltts without flags to use the TUI",
				eng.Name())
		}
		fmt.Fprintf(os.Stderr, "installing %s...\n", eng.Name())
		log := func(s string) { fmt.Fprintln(os.Stderr, "  "+s) }
		if err := eng.Install(context.Background(), log); err != nil {
			return fmt.Errorf("install %s: %w", eng.ID(), err)
		}
		if !eng.Ready() {
			return fmt.Errorf("install %s: completed but engine still not ready", eng.ID())
		}
	}

	// Resolve voice.
	voices := eng.Voices()
	if len(voices) == 0 {
		return fmt.Errorf("%s has no voices available", eng.Name())
	}
	var voice Voice
	if o.voiceID == "" {
		voice = voices[0]
	} else {
		found := false
		for _, v := range voices {
			if v.ID == o.voiceID {
				voice, found = v, true
				break
			}
		}
		if !found {
			return fmt.Errorf("voice %q not found for engine %s (try -list-voices)", o.voiceID, eng.ID())
		}
	}

	// Resolve input + output paths.
	input := expandHome(o.input)
	if fi, err := os.Stat(input); err != nil {
		return fmt.Errorf("input: %w", err)
	} else if fi.IsDir() {
		return fmt.Errorf("input %q is a directory", input)
	}

	out := o.output
	if out == "" {
		out = strings.TrimSuffix(input, filepath.Ext(input)) + ".mp3"
	} else {
		out = expandHome(out)
	}

	raw, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	text := strings.TrimSpace(extractText(string(raw), strings.ToLower(filepath.Ext(input))))
	if text == "" {
		return fmt.Errorf("input produced no text")
	}

	onProg := func(i, total int) {
		fmt.Fprintf(os.Stderr, "\rchunk %d/%d", i, total)
		if i == total {
			fmt.Fprintln(os.Stderr)
		}
	}
	if err := Synthesize(context.Background(), eng, text, voice, o.slow, out, onProg); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "wrote %s\n", out)
	return nil
}

func pickEngine(es []Engine, id string) Engine {
	for _, e := range es {
		if e.ID() == id {
			return e
		}
	}
	return nil
}
