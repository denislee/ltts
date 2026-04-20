package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

var (
	styleTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13"))
	styleInfo  = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	styleErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

// runTUI is the entry point from main. It walks the user through:
// input file -> engine -> install if needed -> voice -> output -> synth.
func runTUI() error {
	fmt.Println(styleTitle.Render("ltts — text-to-speech to MP3"))
	fmt.Println(styleInfo.Render("Ctrl+C to quit at any time."))
	fmt.Println()

	// 1) Input file — list convertible files in the current directory.
	input, err := pickInputFile()
	if err != nil {
		return err
	}

	// 2) Engine select.
	engines := Engines()
	engineOpts := make([]huh.Option[string], 0, len(engines))
	for _, e := range engines {
		label := e.Name()
		if e.Ready() {
			label += "  (installed)"
		}
		engineOpts = append(engineOpts, huh.NewOption(label, e.ID()))
	}
	var engineID string
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("TTS engine").
				Description(engineDescriptions(engines)).
				Options(engineOpts...).
				Value(&engineID),
		),
	).Run(); err != nil {
		return err
	}
	eng := findEngine(engines, engineID)

	// 3) Install if needed.
	if !eng.Ready() {
		var confirm bool
		if err := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title(fmt.Sprintf("Install %s?", eng.Name())).
					Description(eng.Description()).
					Affirmative("Install").
					Negative("Cancel").
					Value(&confirm),
			),
		).Run(); err != nil {
			return err
		}
		if !confirm {
			return fmt.Errorf("install cancelled")
		}
		if err := runInstall(eng); err != nil {
			return err
		}
	}

	// 4) Voice select — dedicated screen.
	voices := eng.Voices()
	if len(voices) == 0 {
		return fmt.Errorf("%s has no voices available", eng.Name())
	}
	voiceOpts := make([]huh.Option[string], 0, len(voices))
	for _, v := range voices {
		voiceOpts = append(voiceOpts, huh.NewOption(v.Label, v.ID))
	}
	voiceID := voices[0].ID
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Voice / language").
				Description(fmt.Sprintf("%d available in %s", len(voices), eng.Name())).
				Options(voiceOpts...).
				Value(&voiceID),
		),
	).Run(); err != nil {
		return err
	}
	voice := findVoice(voices, voiceID)

	// 5) Speed + output path.
	var normal = true
	defaultOut := strings.TrimSuffix(input, filepath.Ext(input)) + ".mp3"
	out := defaultOut
	if err := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Speech speed").
				Affirmative("Normal").
				Negative("Slow").
				Value(&normal),
			huh.NewInput().
				Title("Output MP3").
				Value(&out).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("required")
					}
					return nil
				}),
		),
	).Run(); err != nil {
		return err
	}
	slow := !normal
	out = expandHome(strings.TrimSpace(out))

	// 5) Load + extract + synth.
	raw, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	text := strings.TrimSpace(extractText(string(raw), strings.ToLower(filepath.Ext(input))))
	if text == "" {
		return fmt.Errorf("input produced no text")
	}

	return runSynth(eng, text, voice, slow, out)
}

// supportedExts lists extensions ltts knows how to extract text from.
var supportedExts = map[string]bool{
	".txt":      true,
	".md":       true,
	".markdown": true,
	".html":     true,
	".htm":      true,
	".xhtml":    true,
}

// pickInputFile scans the current directory for convertible files and asks
// the user to pick one. If none are found it falls back to a free-form path.
func pickInputFile() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return "", err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if supportedExts[strings.ToLower(filepath.Ext(e.Name()))] {
			files = append(files, e.Name())
		}
	}

	if len(files) == 0 {
		fmt.Println(styleInfo.Render(
			"No .txt / .md / .html files in " + cwd + " — enter a path instead."))
		var p string
		if err := huh.NewForm(huh.NewGroup(
			huh.NewInput().
				Title("Input file").
				Value(&p).
				Validate(validatePath),
		)).Run(); err != nil {
			return "", err
		}
		return expandHome(strings.TrimSpace(p)), nil
	}

	opts := make([]huh.Option[string], 0, len(files))
	for _, f := range files {
		opts = append(opts, huh.NewOption(f, f))
	}
	var pick string
	if err := huh.NewForm(huh.NewGroup(
		huh.NewSelect[string]().
			Title("Input file").
			Description("Files in " + cwd).
			Options(opts...).
			Value(&pick),
	)).Run(); err != nil {
		return "", err
	}
	return filepath.Join(cwd, pick), nil
}

func validatePath(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		return fmt.Errorf("required")
	}
	fi, err := os.Stat(expandHome(s))
	if err != nil {
		return err
	}
	if fi.IsDir() {
		return fmt.Errorf("is a directory")
	}
	return nil
}

func engineDescriptions(es []Engine) string {
	var b strings.Builder
	for _, e := range es {
		fmt.Fprintf(&b, "%s: %s\n", e.Name(), e.Description())
	}
	return strings.TrimRight(b.String(), "\n")
}

func findEngine(es []Engine, id string) Engine {
	for _, e := range es {
		if e.ID() == id {
			return e
		}
	}
	return es[0]
}

func findVoice(vs []Voice, id string) Voice {
	for _, v := range vs {
		if v.ID == id {
			return v
		}
	}
	if len(vs) > 0 {
		return vs[0]
	}
	return Voice{}
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

// --- install / synth screens (bubbletea) --------------------------------

type jobMsg struct {
	line string
	done bool
	err  error
}

type progressModel struct {
	title string
	lines []string
	done  bool
	err   error
	sub   chan jobMsg
	mu    sync.Mutex
}

func (m *progressModel) Init() tea.Cmd { return waitForJob(m.sub) }

func (m *progressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.done {
			return m, tea.Quit
		}
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case jobMsg:
		if msg.line != "" {
			m.lines = append(m.lines, msg.line)
			if len(m.lines) > 20 {
				m.lines = m.lines[len(m.lines)-20:]
			}
		}
		if msg.done {
			m.done = true
			m.err = msg.err
			return m, nil
		}
		return m, waitForJob(m.sub)
	}
	return m, nil
}

func (m *progressModel) View() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(m.title))
	b.WriteString("\n\n")
	for _, l := range m.lines {
		b.WriteString("  " + l + "\n")
	}
	b.WriteString("\n")
	switch {
	case m.err != nil:
		b.WriteString(styleErr.Render("failed: " + m.err.Error()))
		b.WriteString("\n\npress any key to exit")
	case m.done:
		b.WriteString(styleOK.Render("done"))
		b.WriteString("\n\npress any key to exit")
	default:
		b.WriteString(styleInfo.Render("working..."))
	}
	return b.String()
}

func waitForJob(ch chan jobMsg) tea.Cmd {
	return func() tea.Msg { return <-ch }
}

// runInstall runs eng.Install in a goroutine while bubbletea shows progress.
func runInstall(eng Engine) error {
	ch := make(chan jobMsg, 32)
	m := &progressModel{title: "Installing " + eng.Name(), sub: ch}
	go func() {
		log := func(s string) { ch <- jobMsg{line: s} }
		err := eng.Install(context.Background(), log)
		if err == nil {
			ch <- jobMsg{line: "verifying"}
			if !eng.Ready() {
				err = fmt.Errorf("install completed but engine still not ready")
			}
		}
		ch <- jobMsg{done: true, err: err}
	}()
	prog := tea.NewProgram(m)
	if _, err := prog.Run(); err != nil {
		return err
	}
	return m.err
}

// runSynth runs Synthesize in a goroutine while bubbletea shows progress.
func runSynth(eng Engine, text string, voice Voice, slow bool, out string) error {
	ch := make(chan jobMsg, 32)
	m := &progressModel{title: fmt.Sprintf("Synthesizing with %s → %s", eng.Name(), out), sub: ch}
	go func() {
		onProg := func(i, total int) {
			ch <- jobMsg{line: fmt.Sprintf("chunk %d/%d", i, total)}
		}
		err := Synthesize(context.Background(), eng, text, voice, slow, out, onProg)
		ch <- jobMsg{done: true, err: err}
	}()
	prog := tea.NewProgram(m)
	if _, err := prog.Run(); err != nil {
		return err
	}
	return m.err
}
