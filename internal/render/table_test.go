package render

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/style"
)

var now = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func exit(n int) *int { return &n }

func sample() []job.Job {
	return []job.Job{
		{
			Name: "backup", Source: job.Cron, Kind: job.Scheduled, State: job.StateIdle,
			Schedule: "daily at 03:00", Next: now.Add(15 * time.Hour),
		},
		{
			Name: "com.example.a-very-long-launchd-label-that-goes-on", Source: job.Launchd,
			Kind: job.Scheduled, State: job.StateIdle, LastExit: exit(3),
			Schedule: "every 15 minutes from 09:00 to 17:45 on weekdays", Next: now.Add(4 * time.Minute),
			Problems: []job.Problem{
				{Code: "failed", Severity: job.Fail, Summary: "Last run failed with exit code 3"},
				{Code: "unloaded", Severity: job.Warn, Summary: "Not loaded"},
				{Code: "output", Severity: job.Info, Summary: "Output goes to local mail"},
			},
		},
		{
			Name: "db", Source: job.Launchd, Kind: job.Service, State: job.StateRunning,
			Schedule: "always running",
		},
		{
			Name: "crashy", Source: job.Launchd, Kind: job.Service, State: job.StateIdle,
			Schedule: "at login", LastExit: exit(-11),
		},
	}
}

func render(t *testing.T, jobs []job.Job, width int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := Table(&buf, jobs, Options{Width: width, Now: now, Style: style.New(false)}); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestTable(t *testing.T) {
	out := render(t, sample(), 0)
	for _, want := range []string{
		"Scheduled 2\n",
		"Services 2\n",
		"✕ com.example.a-very-long-launchd-label-that-goes-on",
		"  Last run failed with exit code 3 +1\n",
		"in 4m",
		"exit 3",
		"running",
		"crashed",
		"4 jobs · 1 failing\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "local mail") {
		t.Error("info notes should not appear in the table")
	}

	// Failing jobs come first within their section.
	if strings.Index(out, "com.example") > strings.Index(out, "backup") {
		t.Errorf("failing job should be listed first:\n%s", out)
	}
}

func TestTableFitsWidth(t *testing.T) {
	out := render(t, sample(), 80)
	for line := range strings.SplitSeq(strings.TrimRight(out, "\n"), "\n") {
		if n := utf8.RuneCountInString(line); n > 80 {
			t.Errorf("line is %d wide, want at most 80: %q", n, line)
		}
	}
	if !strings.Contains(out, "…") {
		t.Errorf("long names should be truncated:\n%s", out)
	}
}

func TestTableEmpty(t *testing.T) {
	if out := render(t, nil, 80); out != "No jobs found.\n" {
		t.Errorf("got %q", out)
	}
}

func TestTableHealthy(t *testing.T) {
	jobs := sample()[:1]
	if out := render(t, jobs, 0); !strings.HasSuffix(out, "1 job · all healthy\n") {
		t.Errorf("got %q", out)
	}
}

func TestUntil(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "in <1m"},
		{4 * time.Minute, "in 4m"},
		{90 * time.Minute, "in 1h"},
		{30 * time.Hour, "in 1d"},
	}
	for _, tt := range tests {
		if got := Until(now.Add(tt.d), now); got != tt.want {
			t.Errorf("Until(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		s    string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 4, "hel…"},
		{"hello", 1, "…"},
		{"hello", 0, ""},
	}
	for _, tt := range tests {
		if got := Truncate(tt.s, tt.n); got != tt.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.s, tt.n, got, tt.want)
		}
	}
}

func BenchmarkTable(b *testing.B) {
	jobs := sample()
	for range 50 {
		jobs = append(jobs, sample()...)
	}
	var buf bytes.Buffer
	for b.Loop() {
		buf.Reset()
		_ = Table(&buf, jobs, Options{Width: 120, Now: now, Style: style.New(true)})
	}
}
