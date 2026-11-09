package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/style"
)

func fixture(n int) []job.Job {
	jobs := make([]job.Job, 0, n+2)
	for i := range n {
		jobs = append(jobs, job.Job{
			ID: fmt.Sprintf("id-%02d", i), Name: fmt.Sprintf("job-%02d", i),
			Source: job.Launchd, Kind: job.Scheduled, State: job.StateIdle,
			Schedule: "daily at 09:00", Command: "/usr/bin/true",
		})
	}
	jobs = append(jobs,
		job.Job{
			ID: "broken", Name: "zz-broken", Source: job.Cron, Kind: job.Scheduled, State: job.StateIdle,
			Schedule: "hourly", Command: "/nope",
			Problems: []job.Problem{{Code: "program-missing", Severity: job.Fail, Summary: "Program not found: /nope", Fix: "Fix the path"}},
		},
		job.Job{ID: "db", Name: "db", Source: job.Launchd, Kind: job.Service, State: job.StateRunning, Schedule: "always running"},
	)
	return jobs
}

func newModel(t *testing.T, jobs []job.Job, w, h int) *model {
	t.Helper()
	m := New(func(bool) []job.Job { return jobs }, jobs, false, style.New(false)).(*model)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func press(m *model, keys ...string) {
	for _, k := range keys {
		var msg tea.KeyPressMsg
		switch k {
		case "down":
			msg = tea.KeyPressMsg{Code: tea.KeyDown}
		case "up":
			msg = tea.KeyPressMsg{Code: tea.KeyUp}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "backspace":
			msg = tea.KeyPressMsg{Code: tea.KeyBackspace}
		default:
			r, _ := utf8.DecodeRuneInString(k)
			msg = tea.KeyPressMsg{Code: r, Text: k}
		}
		m.Update(msg)
	}
}

func TestBrokenJobsFirst(t *testing.T) {
	m := newModel(t, fixture(3), 80, 30)
	if got := m.selected().ID; got != "broken" {
		t.Errorf("first selected = %q, want the broken job", got)
	}
}

func TestMove(t *testing.T) {
	m := newModel(t, fixture(3), 80, 30)
	press(m, "down", "down")
	if got := m.selected().ID; got != "id-01" {
		t.Errorf("selected = %q, want id-01", got)
	}
	press(m, "G")
	if got := m.selected().ID; got != "db" {
		t.Errorf("after G selected = %q, want db", got)
	}
	press(m, "g")
	if m.cursor != 0 {
		t.Errorf("after g cursor = %d, want 0", m.cursor)
	}
	press(m, "up")
	if m.cursor != 0 {
		t.Errorf("cursor moved above the first row: %d", m.cursor)
	}
}

func TestFilter(t *testing.T) {
	m := newModel(t, fixture(5), 80, 30)
	press(m, "/", "j", "o", "b", "-", "0", "3")
	if len(m.rows) != 1 || m.selected().ID != "id-03" {
		t.Fatalf("filter left %d rows, selected %q", len(m.rows), m.selected().ID)
	}
	press(m, "backspace", "enter")
	if m.typing || m.filter != "job-0" || len(m.rows) != 5 {
		t.Errorf("typing=%v filter=%q rows=%d", m.typing, m.filter, len(m.rows))
	}
	press(m, "esc")
	if m.filter != "" || len(m.rows) != 7 {
		t.Errorf("esc should clear the filter: filter=%q rows=%d", m.filter, len(m.rows))
	}
}

func TestFilterNoMatches(t *testing.T) {
	m := newModel(t, fixture(3), 80, 30)
	press(m, "/", "x", "y", "z")
	if !strings.Contains(m.View().Content, `No jobs match "xyz".`) {
		t.Errorf("missing empty message:\n%s", m.View().Content)
	}
	press(m, "enter")
	if m.detail {
		t.Error("enter with no rows should not open the detail view")
	}
}

func TestReloadKeepsSelection(t *testing.T) {
	jobs := fixture(5)
	m := newModel(t, jobs, 80, 30)
	press(m, "down", "down", "down")
	want := m.selected().ID

	// The broken job is fixed, so every row shifts up by one.
	fixed := fixture(5)
	fixed[5].Problems = nil
	m.Update(loadedMsg{jobs: fixed})
	if got := m.selected().ID; got != want {
		t.Errorf("selected = %q after reload, want %q", got, want)
	}
}

func TestReloadIgnoresStaleResults(t *testing.T) {
	m := newModel(t, fixture(3), 80, 30)
	m.Update(loadedMsg{jobs: nil, all: true})
	if len(m.jobs) == 0 {
		t.Error("a result for the other view replaced the jobs")
	}
}

func TestDetail(t *testing.T) {
	m := newModel(t, fixture(3), 80, 30)
	press(m, "enter")
	if !m.detail {
		t.Fatal("enter should open the detail view")
	}
	out := m.View().Content
	for _, want := range []string{"zz-broken", "command  /nope", "✕ Program not found: /nope", "Fix the path"} {
		if !strings.Contains(out, want) {
			t.Errorf("detail is missing %q:\n%s", want, out)
		}
	}
	press(m, "esc")
	if m.detail {
		t.Error("esc should close the detail view")
	}
}

// Every frame must fill the window exactly, or the terminal scrolls.
func TestViewFitsWindow(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {60, 12}, {120, 50}, {40, 8}} {
		w, h := size[0], size[1]
		m := newModel(t, fixture(40), w, h)
		for _, keys := range [][]string{nil, {"G"}, {"enter"}, {"/", "j"}} {
			press(m, keys...)
			lines := strings.Split(m.View().Content, "\n")
			if len(lines) != h {
				t.Errorf("%dx%d after %v: %d lines, want %d", w, h, keys, len(lines), h)
			}
			for _, l := range lines {
				if n := utf8.RuneCountInString(l); n > w {
					t.Errorf("%dx%d after %v: line is %d wide: %q", w, h, keys, n, l)
				}
			}
			press(m, "esc", "esc")
		}
	}
}

func TestSelectionStaysVisible(t *testing.T) {
	m := newModel(t, fixture(40), 80, 20)
	for range 30 {
		press(m, "down")
	}
	if !strings.Contains(m.View().Content, "▌ ● "+m.selected().Name) {
		t.Errorf("selected row %q is off screen:\n%s", m.selected().Name, m.View().Content)
	}
}

func TestStatus(t *testing.T) {
	segv, fail := -11, 3
	tests := []struct {
		j    job.Job
		want string
	}{
		{job.Job{Kind: job.Scheduled, State: job.StateIdle, LastExit: &segv}, "waiting · last run killed (segmentation fault)"},
		{job.Job{Kind: job.Service, State: job.StateIdle, LastExit: &fail}, "stopped · last exit 3"},
		{job.Job{Kind: job.Service, State: job.StateRunning, PID: 42}, "running · pid 42"},
	}
	for _, tt := range tests {
		if got := status(&tt.j); got != tt.want {
			t.Errorf("status = %q, want %q", got, tt.want)
		}
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.Local)
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "just now"},
		{12 * time.Minute, "12m ago"},
		{5 * time.Hour, "5h ago"},
		{3 * 24 * time.Hour, "3d ago"},
		{20 * 24 * time.Hour, "Oct 20"},
	}
	for _, tt := range tests {
		if got := ago(now.Add(-tt.d), now); got != tt.want {
			t.Errorf("ago(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestWhen(t *testing.T) {
	now := time.Date(2026, 11, 8, 21, 0, 0, 0, time.Local) // a Sunday
	tests := []struct {
		t    time.Time
		want string
	}{
		{time.Date(2026, 11, 8, 23, 30, 0, 0, time.Local), "today 23:30"},
		{time.Date(2026, 11, 9, 9, 0, 0, 0, time.Local), "tomorrow 09:00"},
		{time.Date(2026, 11, 12, 9, 0, 0, 0, time.Local), "Thu 09:00"},
		{time.Date(2026, 12, 1, 3, 0, 0, 0, time.Local), "Dec 1 03:00"},
	}
	for _, tt := range tests {
		if got := when(tt.t, now); got != tt.want {
			t.Errorf("when(%v) = %q, want %q", tt.t, got, tt.want)
		}
	}
}
