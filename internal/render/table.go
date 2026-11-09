// Package render prints jobs for people and for programs.
package render

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/style"
)

// Options controls how the table is laid out.
type Options struct {
	// Width is the terminal width. Zero means no limit.
	Width int
	Now   time.Time
	Style style.Style
}

const (
	gap         = "  "
	minName     = 16
	minSchedule = 14
)

type row struct {
	job                     *job.Job
	glyph                   string
	name, sched, when, last string
	source                  string

	glyphStyle, whenStyle, lastStyle func(string) string
}

// Table writes jobs grouped into scheduled jobs and services, with broken
// jobs first in each group and their worst problem underneath.
func Table(w io.Writer, jobs []job.Job, o Options) error {
	s := o.Style
	if len(jobs) == 0 {
		_, err := fmt.Fprintln(w, s.Dim("No jobs found."))
		return err
	}

	var scheduled, services []row
	for i := range jobs {
		r := newRow(&jobs[i], o)
		if jobs[i].Kind == job.Scheduled {
			scheduled = append(scheduled, r)
		} else {
			services = append(services, r)
		}
	}
	sortRows(scheduled)
	sortRows(services)

	widths := layout(append(slices.Clone(scheduled), services...), o.Width)

	var b strings.Builder
	section := func(title string, rows []row) {
		if len(rows) == 0 {
			return
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s %s\n", s.Bold(title), s.Dim(fmt.Sprint(len(rows))))
		for _, r := range rows {
			writeRow(&b, r, widths, o)
		}
	}
	section("Scheduled", scheduled)
	section("Services", services)
	b.WriteByte('\n')
	b.WriteString(summary(jobs, s))
	b.WriteByte('\n')

	_, err := io.WriteString(w, b.String())
	return err
}

func newRow(j *job.Job, o Options) row {
	s := o.Style
	r := row{job: j, name: j.Name, sched: j.Schedule, source: string(j.Source), whenStyle: s.Dim, lastStyle: s.Dim}

	switch worst := j.Worst(); {
	case worst == job.Fail:
		r.glyph, r.glyphStyle = "✕", s.Red
	case worst == job.Warn:
		r.glyph, r.glyphStyle = "▲", s.Yellow
	case j.State == job.StateUnloaded || j.State == job.StateDisabled:
		r.glyph, r.glyphStyle = "○", s.Dim
	default:
		r.glyph, r.glyphStyle = "●", s.Green
	}

	switch {
	case j.State == job.StateDisabled:
		r.when = "disabled"
	case j.State == job.StateUnloaded:
		r.when = "not loaded"
	case j.State == job.StateRunning:
		r.when, r.whenStyle = "running", s.Green
	case j.Kind == job.Service && j.State == job.StateIdle:
		r.when = "stopped"
	case !j.Next.IsZero():
		r.when, r.whenStyle = until(j.Next, o.Now), func(t string) string { return t }
	default:
		r.when = "–"
	}

	switch code := j.LastExit; {
	case code == nil:
		r.last = "–"
	case *code == 0:
		r.last = "ok"
	case *code < 0:
		sig := syscall.Signal(-*code)
		if sig == syscall.SIGKILL || sig == syscall.SIGTERM {
			r.last, r.lastStyle = "killed", s.Yellow
		} else {
			r.last, r.lastStyle = "crashed", s.Red
		}
	default:
		r.last, r.lastStyle = fmt.Sprintf("exit %d", *code), s.Red
	}
	return r
}

// rank orders rows by what the table shows. Info notes are not shown, so
// they don't count.
func rank(j *job.Job) job.Severity {
	if w := j.Worst(); w >= job.Warn {
		return w
	}
	return -1
}

func sortRows(rows []row) {
	slices.SortStableFunc(rows, func(a, b row) int {
		if c := cmp.Compare(rank(b.job), rank(a.job)); c != 0 {
			return c
		}
		return cmp.Compare(a.name, b.name)
	})
}

type widths struct{ name, sched, when, last, source int }

// layout sizes the columns to their content, then shrinks the name and
// schedule columns to fit the terminal.
func layout(rows []row, termWidth int) widths {
	var w widths
	for _, r := range rows {
		w.name = max(w.name, width(r.name))
		w.sched = max(w.sched, width(r.sched))
		w.when = max(w.when, width(r.when))
		w.last = max(w.last, width(r.last))
		w.source = max(w.source, width(r.source))
	}
	if termWidth <= 0 {
		return w
	}

	// glyph, space, then five columns separated by gaps
	fixed := 2 + w.when + w.last + w.source + 4*len(gap)
	over := fixed + w.name + w.sched - termWidth
	for over > 0 {
		switch {
		case w.name > minName && w.name >= w.sched:
			w.name--
		case w.sched > minSchedule:
			w.sched--
		case w.name > minName:
			w.name--
		default:
			return w
		}
		over--
	}
	return w
}

func writeRow(b *strings.Builder, r row, w widths, o Options) {
	s := o.Style
	b.WriteString(r.glyphStyle(r.glyph))
	b.WriteByte(' ')
	b.WriteString(pad(truncate(r.name, w.name), w.name))
	b.WriteString(gap)
	b.WriteString(s.Dim(pad(truncate(r.sched, w.sched), w.sched)))
	b.WriteString(gap)
	b.WriteString(r.whenStyle(pad(r.when, w.when)))
	b.WriteString(gap)
	b.WriteString(r.lastStyle(pad(r.last, w.last)))
	b.WriteString(gap)
	b.WriteString(s.Dim(r.source))
	b.WriteByte('\n')

	p, more := headline(r.job)
	if p == nil {
		return
	}
	color := s.Yellow
	if p.Severity == job.Fail {
		color = s.Red
	}
	line := p.Summary
	if o.Width > 0 {
		line = truncate(line, o.Width-2)
	}
	b.WriteString("  ")
	b.WriteString(color(line))
	if more > 0 {
		b.WriteString(s.Dim(fmt.Sprintf(" +%d", more)))
	}
	b.WriteByte('\n')
}

// headline returns the worst warning or failure, and how many more there are.
// Notes at info level are left for the detail view.
func headline(j *job.Job) (*job.Problem, int) {
	var serious int
	for _, p := range j.Problems {
		if p.Severity >= job.Warn {
			serious++
		}
	}
	if serious == 0 {
		return nil, 0
	}
	return &j.Problems[0], serious - 1
}

func summary(jobs []job.Job, s style.Style) string {
	var fail, warn int
	for i := range jobs {
		switch jobs[i].Worst() {
		case job.Fail:
			fail++
		case job.Warn:
			warn++
		}
	}
	parts := []string{plural(len(jobs), "job")}
	if fail > 0 {
		parts = append(parts, s.Red(fmt.Sprintf("%d failing", fail)))
	}
	if warn > 0 {
		parts = append(parts, s.Yellow(plural(warn, "warning")))
	}
	if fail == 0 && warn == 0 {
		parts = append(parts, s.Green("all healthy"))
	}
	return strings.Join(parts, s.Dim(" · "))
}

// until formats the time to the next run, such as "in 4m" or "in 2d".
func until(t, now time.Time) string {
	d := t.Sub(now)
	switch {
	case d < time.Minute:
		return "in <1m"
	case d < time.Hour:
		return fmt.Sprintf("in %dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("in %dh", int(d.Hours()))
	}
	return fmt.Sprintf("in %dd", int(d.Hours()/24))
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func width(s string) int { return utf8.RuneCountInString(s) }

func pad(s string, n int) string {
	if w := width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

func truncate(s string, n int) string {
	if width(s) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
