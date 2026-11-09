package render

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/style"
)

// Tone is the color a cell is drawn in.
type Tone int

const (
	Plain Tone = iota
	Muted
	Good
	Caution
	Bad
)

// Paint draws text in a tone.
func Paint(s style.Style, t Tone, text string) string {
	switch t {
	case Muted:
		return s.Dim(text)
	case Good:
		return s.Green(text)
	case Caution:
		return s.Yellow(text)
	case Bad:
		return s.Red(text)
	}
	return text
}

// Row is one job as a line of cells.
type Row struct {
	Job *job.Job

	Glyph, Name, Schedule, When, Last, Source string

	GlyphTone, WhenTone, LastTone Tone
}

// NewRow describes a job's status in short cells, such as "in 4m" and
// "exit 3".
func NewRow(j *job.Job, now time.Time) Row {
	r := Row{
		Job: j, Name: j.Name, Schedule: j.Schedule, Source: string(j.Source),
		WhenTone: Muted, LastTone: Muted,
	}

	switch worst := j.Worst(); {
	case worst == job.Fail:
		r.Glyph, r.GlyphTone = "✕", Bad
	case worst == job.Warn:
		r.Glyph, r.GlyphTone = "▲", Caution
	case j.State == job.StateUnloaded || j.State == job.StateDisabled:
		r.Glyph, r.GlyphTone = "○", Muted
	default:
		r.Glyph, r.GlyphTone = "●", Good
	}

	switch {
	case j.State == job.StateDisabled:
		r.When = "disabled"
	case j.State == job.StateUnloaded:
		r.When = "not loaded"
	case j.State == job.StateRunning:
		r.When, r.WhenTone = "running", Good
	case j.Kind == job.Service && j.State == job.StateIdle:
		r.When = "stopped"
	case !j.Next.IsZero():
		r.When, r.WhenTone = Until(j.Next, now), Plain
	default:
		r.When = "–"
	}

	switch code := j.LastExit; {
	case code == nil && j.LastResult != "":
		r.Last, r.LastTone = lastResult(j)
	case code == nil:
		r.Last = "–"
	case *code == 0:
		r.Last = "ok"
	case *code < 0:
		sig := syscall.Signal(-*code)
		if sig == syscall.SIGKILL || sig == syscall.SIGTERM {
			r.Last, r.LastTone = "killed", Caution
		} else {
			r.Last, r.LastTone = "crashed", Bad
		}
	default:
		r.Last, r.LastTone = fmt.Sprintf("exit %d", *code), Bad
	}
	return r
}

// lastResult shortens how an agent scheduler described the last run.
func lastResult(j *job.Job) (string, Tone) {
	switch {
	case j.LastResult == "ok" || j.LastResult == "delivery_queued":
		return "ok", Muted
	case j.Source == job.Claude:
		return "skipped", Bad
	}
	return "failed", Bad
}

// Group splits jobs into scheduled jobs and services, each with broken jobs
// first and then by name.
func Group(jobs []job.Job, now time.Time) (scheduled, services []Row) {
	for i := range jobs {
		r := NewRow(&jobs[i], now)
		if jobs[i].Kind == job.Scheduled {
			scheduled = append(scheduled, r)
		} else {
			services = append(services, r)
		}
	}
	sortRows(scheduled)
	sortRows(services)
	return scheduled, services
}

// rank orders rows by what a list shows. Info notes are not shown, so they
// don't count.
func rank(j *job.Job) job.Severity {
	if w := j.Worst(); w >= job.Warn {
		return w
	}
	return -1
}

func sortRows(rows []Row) {
	slices.SortStableFunc(rows, func(a, b Row) int {
		if c := cmp.Compare(rank(b.Job), rank(a.Job)); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})
}

const (
	gap         = "  "
	roomyName   = 24
	minName     = 16
	minSchedule = 14
	tiny        = 6
)

// Widths are column widths shared by every row in a list.
type Widths struct{ Name, Schedule, When, Last, Source int }

// Fit sizes columns to their content, then shrinks the name and schedule
// columns until a row fits in width, dropping the source and last-run
// columns if it still doesn't. Zero width means no limit. indent is the
// space taken before the glyph.
func Fit(rows []Row, width, indent int) Widths {
	var w Widths
	for _, r := range rows {
		w.Name = max(w.Name, Width(r.Name))
		w.Schedule = max(w.Schedule, Width(r.Schedule))
		w.When = max(w.When, Width(r.When))
		w.Last = max(w.Last, Width(r.Last))
		w.Source = max(w.Source, Width(r.Source))
	}
	if width <= 0 {
		return w
	}

	// Give up space in order of usefulness: long schedules, long names, the
	// source column, then everything down to a bare minimum.
	for w.total(indent) > width {
		switch {
		case w.Schedule > max(minSchedule, w.Name):
			w.Schedule--
		case w.Name > roomyName:
			w.Name--
		case w.Source > 0:
			w.Source = 0
		case w.Schedule > minSchedule:
			w.Schedule--
		case w.Name > minName:
			w.Name--
		case w.Last > 0:
			w.Last = 0
		case w.Schedule > tiny:
			w.Schedule--
		case w.Name > tiny:
			w.Name--
		default:
			return w
		}
	}
	return w
}

// total is the width of a full row. Hidden columns have zero width.
func (w Widths) total(indent int) int {
	n := indent + 2 + w.Name + len(gap) + w.Schedule + len(gap) + w.When
	for _, c := range []int{w.Last, w.Source} {
		if c > 0 {
			n += len(gap) + c
		}
	}
	return n
}

// Line draws a row. A bold row has its name in bold, for the selected row.
func (r Row) Line(w Widths, s style.Style, bold bool) string {
	type cell struct {
		text  string
		width int
		paint func(string) string
	}
	name := func(t string) string { return t }
	if bold {
		name = s.Bold
	}
	tone := func(t Tone) func(string) string {
		return func(text string) string { return Paint(s, t, text) }
	}
	cells := []cell{
		{Truncate(r.Name, w.Name), w.Name, name},
		{Truncate(r.Schedule, w.Schedule), w.Schedule, s.Dim},
		{r.When, w.When, tone(r.WhenTone)},
	}
	if w.Last > 0 {
		cells = append(cells, cell{r.Last, w.Last, tone(r.LastTone)})
	}
	if w.Source > 0 {
		cells = append(cells, cell{r.Source, w.Source, s.Dim})
	}

	var b strings.Builder
	b.WriteString(Paint(s, r.GlyphTone, r.Glyph))
	for i, c := range cells {
		text := c.text
		// The last column is not padded, so lines never end in spaces.
		if i < len(cells)-1 {
			text = Pad(text, c.width)
		}
		if i == 0 {
			b.WriteByte(' ')
		} else {
			b.WriteString(gap)
		}
		b.WriteString(c.paint(text))
	}
	return b.String()
}

// Summary counts jobs and their problems: "31 jobs · 10 failing".
func Summary(jobs []job.Job, s style.Style) string {
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
	if fail == 0 && warn == 0 && len(jobs) > 0 {
		parts = append(parts, s.Green("all healthy"))
	}
	return strings.Join(parts, s.Dim(" · "))
}

// Until formats the time to the next run, such as "in 4m" or "in 2d".
func Until(t, now time.Time) string {
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

// Width is the number of terminal cells s takes, assuming no wide runes.
func Width(s string) int { return utf8.RuneCountInString(s) }

// Pad fills s with spaces to n cells.
func Pad(s string, n int) string {
	if w := Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return s
}

// Truncate shortens s to n cells, ending with an ellipsis when cut.
func Truncate(s string, n int) string {
	if Width(s) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}
