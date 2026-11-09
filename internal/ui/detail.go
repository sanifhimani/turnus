package ui

import (
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/render"
)

const labelWidth = 9

// describe lays out everything known about a job, one field per line, with
// problems and their fixes last.
func (m *model) describe(j *job.Job) []string {
	s := m.style
	valueWidth := max(m.width-labelWidth-4, 10)

	var lines []string
	field := func(label, value string) {
		if value == "" {
			return
		}
		lines = append(lines, "  "+s.Dim(render.Pad(label, labelWidth))+render.Truncate(value, valueWidth))
	}

	title := "  " + s.Bold(render.Truncate(j.Name, m.width-20)) + "  " + s.Dim(string(j.Source)+" · "+string(j.Scope))
	lines = append(lines, title)

	runs := j.Schedule
	if !j.Next.IsZero() {
		runs += " · next " + when(j.Next, m.now)
	}
	if j.Expr != "" && j.Expr != j.Schedule {
		runs += " · " + j.Expr
	}
	field("runs", runs)
	field("command", j.Command)
	field("status", status(j))
	file := tilde(j.File)
	if j.Line > 0 {
		file += fmt.Sprintf(":%d", j.Line)
	}
	field("file", file)
	field("log", tilde(j.LogPath))

	for _, p := range j.Problems {
		glyph, tone := "·", render.Muted
		switch p.Severity {
		case job.Fail:
			glyph, tone = "✕", render.Bad
		case job.Warn:
			glyph, tone = "▲", render.Caution
		}
		lines = append(lines, "")
		lines = append(lines, "  "+render.Paint(s, tone, glyph+" "+render.Truncate(p.Summary, m.width-6)))
		if p.Fix != "" {
			lines = append(lines, "    "+s.Dim(render.Truncate(p.Fix, m.width-6)))
		}
	}
	return lines
}

func status(j *job.Job) string {
	var parts []string
	switch j.State {
	case job.StateRunning:
		parts = append(parts, "running")
	case job.StateIdle:
		if j.Kind == job.Service {
			parts = append(parts, "stopped")
		} else {
			parts = append(parts, "waiting")
		}
	case job.StateUnloaded:
		parts = append(parts, "not loaded")
	case job.StateDisabled:
		parts = append(parts, "disabled")
	}
	if j.PID > 0 {
		parts = append(parts, fmt.Sprintf("pid %d", j.PID))
	}
	if j.LastExit != nil {
		parts = append(parts, fmt.Sprintf("last exit %d", *j.LastExit))
	}
	if j.Source == job.Cron {
		parts = append(parts, "cron keeps no run history")
	}
	return strings.Join(parts, " · ")
}

// when formats an upcoming time relative to today: "today 14:00", "tomorrow
// 09:00", "Mon 09:00", or "Jan 2 09:00".
func when(t, now time.Time) string {
	t = t.Local()
	y1, m1, d1 := now.Date()
	y2, m2, d2 := t.Date()
	today := time.Date(y1, m1, d1, 0, 0, 0, 0, time.Local)
	day := time.Date(y2, m2, d2, 0, 0, 0, 0, time.Local)
	days := int(math.Round(day.Sub(today).Hours() / 24)) // days can be 23 or 25 hours

	clock := t.Format("15:04")
	switch {
	case days == 0:
		return "today " + clock
	case days == 1:
		return "tomorrow " + clock
	case days < 7:
		return t.Format("Mon ") + clock
	}
	return t.Format("Jan 2 ") + clock
}

var home, _ = os.UserHomeDir()

func tilde(path string) string {
	if home != "" && strings.HasPrefix(path, home+"/") {
		return "~" + path[len(home):]
	}
	return path
}
