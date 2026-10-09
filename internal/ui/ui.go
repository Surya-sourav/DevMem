package ui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
)

var (
	accent  = lipgloss.Color("#7c6cfc")
	green   = lipgloss.Color("#28c840")
	amber   = lipgloss.Color("#febc2e")
	red     = lipgloss.Color("#ff5f57")
	subtle  = lipgloss.Color("#888888")
	surface = lipgloss.Color("#3a3a3a")

	Accent = lipgloss.NewStyle().Foreground(accent)
	Bold   = lipgloss.NewStyle().Bold(true)
	Muted  = lipgloss.NewStyle().Foreground(subtle)
	Code   = lipgloss.NewStyle().Foreground(accent)

	okMark   = lipgloss.NewStyle().Foreground(green).Render("✓")
	warnMark = lipgloss.NewStyle().Foreground(amber).Render("!")
	failMark = lipgloss.NewStyle().Foreground(red).Render("✗")
	infoMark = lipgloss.NewStyle().Foreground(accent).Render("›")

	badge = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#ffffff")).
		Background(accent).
		Bold(true).
		Padding(0, 1)

	box = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(surface).
		Padding(0, 2).
		MarginLeft(2)
)

// ErrCancelled is returned when the user backs out of a prompt.
var ErrCancelled = errors.New("cancelled")

// Animated reports whether stderr is a terminal that can show spinners.
func Animated() bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return term.IsTerminal(int(os.Stderr.Fd()))
}

// Interactive reports whether full-screen prompts can be shown.
func Interactive() bool {
	return Animated() && stdinIsTerminal()
}

// Secret hides what is typed into the input when there is a terminal to hide it on.
func Secret(input *huh.Input) *huh.Input {
	if stdinIsTerminal() {
		return input.EchoMode(huh.EchoModePassword)
	}
	return input
}

func stdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// lineReader hands out stdin one line per Read. huh's plain prompts wrap
// the reader in a fresh bufio.Scanner each time, which would otherwise
// swallow the answers meant for the following prompts.
type lineReader struct {
	r io.Reader
}

func (l lineReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		m, err := l.r.Read(p[n : n+1])
		n += m
		if err != nil {
			return n, err
		}
		if m == 1 && p[n-1] == '\n' {
			break
		}
	}
	return n, nil
}

func eprintln(a ...any) {
	_, _ = lipgloss.Fprintln(os.Stderr, a...)
}

// Header prints the command banner.
func Header(command, detail string) {
	eprintln()
	line := "  " + badge.Render("devmem") + " " + Bold.Render(command)
	if detail != "" {
		line += "  " + Muted.Render(detail)
	}
	eprintln(line)
	eprintln()
}

// Success prints a completed step.
func Success(msg, detail string) {
	eprintln(row(okMark, msg, detail))
}

// Warn prints a non-fatal problem.
func Warn(msg, detail string) {
	eprintln(row(warnMark, msg, detail))
}

// Info prints a neutral status line.
func Info(msg, detail string) {
	eprintln(row(infoMark, msg, detail))
}

// Error prints a fatal error.
func Error(err error) {
	eprintln()
	if errors.Is(err, ErrCancelled) {
		eprintln(row(Muted.Render("·"), Muted.Render("Cancelled."), ""))
		eprintln()
		return
	}
	eprintln(row(failMark, lipgloss.NewStyle().Foreground(red).Bold(true).Render("Error"), ""))
	for _, line := range strings.Split(strings.TrimSpace(err.Error()), "\n") {
		eprintln("    " + line)
	}
	eprintln()
}

// Stdout prints a line to stdout, dropping colors when it is not a terminal.
func Stdout(s string) {
	_, _ = lipgloss.Fprintln(os.Stdout, s)
}

// OK, Pending and Failed are status marks for custom rows.
func OK() string      { return okMark }
func Pending() string { return lipgloss.NewStyle().Foreground(amber).Render("●") }
func Failed() string  { return failMark }

// Blank prints an empty line.
func Blank() {
	eprintln()
}

// Println prints an indented line.
func Println(s string) {
	eprintln("  " + s)
}

// Box prints a bordered panel with a title and body lines.
func Box(title string, lines ...string) {
	content := Bold.Render(title)
	if len(lines) > 0 {
		content += "\n\n" + strings.Join(lines, "\n")
	}
	eprintln(box.Render(content))
	eprintln()
}

func row(mark, msg, detail string) string {
	s := "  " + mark + " " + msg
	if detail != "" {
		s += "  " + Muted.Render(detail)
	}
	return s
}

func theme() huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		t := huh.ThemeCharm(isDark)
		t.Focused.Title = t.Focused.Title.Foreground(accent)
		t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(accent)
		t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(accent)
		t.Focused.FocusedButton = t.Focused.FocusedButton.Background(accent)
		t.Focused.Next = t.Focused.FocusedButton
		t.Blurred.Title = t.Focused.Title
		t.Group.Title = t.Focused.Title
		return t
	})
}

// Ask runs a prompt form. In non-interactive shells it falls back to
// plain line-based prompts so piping answers still works.
func Ask(fields ...huh.Field) error {
	form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(theme())
	if Interactive() {
		form = form.WithOutput(os.Stderr)
	} else {
		var in io.Reader = os.Stdin
		if !stdinIsTerminal() {
			in = lineReader{os.Stdin}
		}
		form = form.
			WithAccessible(true).
			WithInput(in).
			WithOutput(colorprofile.NewWriter(os.Stderr, os.Environ()))
	}
	err := form.Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrCancelled
	}
	if err != nil {
		return fmt.Errorf("prompt: %w", err)
	}
	return nil
}
