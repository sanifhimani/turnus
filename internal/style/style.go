// Package style colors terminal output with the terminal's own palette, so
// it matches light and dark themes without asking the terminal anything.
package style

import (
	"os"

	"golang.org/x/term"
)

// Style applies ANSI styles when enabled, and passes text through otherwise.
type Style struct {
	enabled bool
}

// New returns a Style that colors output only when it is turned on.
func New(enabled bool) Style { return Style{enabled: enabled} }

// For returns a Style suited to f: colored when f is a terminal, unless
// NO_COLOR is set or TERM is "dumb".
func For(f *os.File) Style {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return New(false)
	}
	return New(term.IsTerminal(int(f.Fd())))
}

func (s Style) Enabled() bool { return s.enabled }

func (s Style) wrap(code, text string) string {
	if !s.enabled || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s Style) Bold(t string) string   { return s.wrap("1", t) }
func (s Style) Dim(t string) string    { return s.wrap("2", t) }
func (s Style) Red(t string) string    { return s.wrap("31", t) }
func (s Style) Green(t string) string  { return s.wrap("32", t) }
func (s Style) Yellow(t string) string { return s.wrap("33", t) }
func (s Style) Cyan(t string) string   { return s.wrap("36", t) }
