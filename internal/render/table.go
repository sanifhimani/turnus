// Package render prints jobs for people and for programs.
package render

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

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

// Table writes jobs grouped into scheduled jobs and services, with broken
// jobs first in each group and their worst problem underneath.
func Table(w io.Writer, jobs []job.Job, o Options) error {
	s := o.Style
	if len(jobs) == 0 {
		_, err := fmt.Fprintln(w, s.Dim("No jobs found."))
		return err
	}

	scheduled, services := Group(jobs, o.Now)
	widths := Fit(slices.Concat(scheduled, services), o.Width, 0)

	var b strings.Builder
	section := func(title string, rows []Row) {
		if len(rows) == 0 {
			return
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "%s %s\n", s.Bold(title), s.Dim(fmt.Sprint(len(rows))))
		for _, r := range rows {
			b.WriteString(r.Line(widths, s, false))
			b.WriteByte('\n')
			writeHeadline(&b, r.Job, o)
		}
	}
	section("Scheduled", scheduled)
	section("Services", services)
	b.WriteByte('\n')
	b.WriteString(Summary(jobs, s))
	b.WriteByte('\n')

	_, err := io.WriteString(w, b.String())
	return err
}

func writeHeadline(b *strings.Builder, j *job.Job, o Options) {
	p, more := Headline(j)
	if p == nil {
		return
	}
	s := o.Style
	line := p.Summary
	if o.Width > 0 {
		line = Truncate(line, o.Width-2)
	}
	b.WriteString("  ")
	b.WriteString(Paint(s, Severity(p.Severity), line))
	if more > 0 {
		b.WriteString(s.Dim(fmt.Sprintf(" +%d", more)))
	}
	b.WriteByte('\n')
}

// Headline returns the worst warning or failure, and how many more there
// are. Notes at info level are left for the detail view.
func Headline(j *job.Job) (*job.Problem, int) {
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

// Severity is the tone a problem is drawn in.
func Severity(sev job.Severity) Tone {
	switch sev {
	case job.Fail:
		return Bad
	case job.Warn:
		return Caution
	}
	return Muted
}
