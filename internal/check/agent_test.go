package check

import (
	"slices"
	"testing"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

func TestAgentChecks(t *testing.T) {
	next := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		job    job.Job
		claude bool // whether Claude Desktop is running
		want   []string
	}{
		{
			name:   "claude task skipped for a missing folder",
			job:    job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Host: "Claude Desktop", Next: next, LastResult: "folder_missing"},
			claude: true,
			want:   []string{"skipped", "needs-host"},
		},
		{
			name: "claude task while the app is closed",
			job:  job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Host: "Claude Desktop", Next: next},
			want: []string{"host-closed"},
		},
		{
			name: "manual claude task doesn't need the app",
			job:  job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Host: "Claude Desktop", Schedule: "manual"},
			want: nil,
		},
		{
			name: "paused claude task is quiet",
			job:  job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateDisabled, Host: "Claude Desktop", LastResult: "sign_in_required"},
			want: nil,
		},
		{
			name: "hermes job that failed",
			job:  job.Job{Source: job.Hermes, Kind: job.Scheduled, State: job.StateIdle, Host: "the Hermes gateway", LastResult: "error"},
			want: []string{"failed", "needs-host"},
		},
		{
			name: "hermes job that couldn't deliver",
			job:  job.Job{Source: job.Hermes, Kind: job.Scheduled, State: job.StateIdle, Host: "the Hermes gateway", LastResult: "delivery_failed"},
			want: []string{"undelivered", "needs-host"},
		},
		{
			name: "codex scheduled task",
			job:  job.Job{Source: job.Codex, Kind: job.Scheduled, State: job.StateIdle, Host: "the ChatGPT desktop app", Next: next},
			want: []string{"needs-host"},
		},
		{
			name:   "claude task with an invalid schedule",
			job:    job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Host: "Claude Desktop", Expr: "0 9 * * 8"},
			claude: true,
			want:   []string{"invalid-schedule"},
		},
		{
			name: "hermes job with an invalid schedule",
			job:  job.Job{Source: job.Hermes, Kind: job.Scheduled, State: job.StateIdle, Host: "the Hermes gateway", Expr: "every tuesday"},
			want: []string{"invalid-schedule", "needs-host"},
		},
		{
			name: "agent store in an unknown format",
			job:  job.Job{Source: job.Codex, Kind: job.Scheduled, Err: "its format isn't one turnus knows"},
			want: []string{"unknown-format"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testEnv(nil)
			env.Running = func(name string) bool { return name == "Claude" && tt.claude }
			jobs := []job.Job{tt.job}
			Run(jobs, env)
			if got := codes(jobs[0]); !slices.Equal(got, tt.want) {
				t.Errorf("problems = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClaudeSkipMessage(t *testing.T) {
	j := &job.Job{Source: job.Claude, LastResult: "sign_in_required"}
	p := lastResult(j, testEnv(nil))
	if p == nil || p.Summary != "Claude Desktop skipped its last run because you were signed out" || p.Fix != "Sign in to Claude Desktop" {
		t.Errorf("got %+v", p)
	}
	j.LastResult = "something_new"
	if p := lastResult(j, testEnv(nil)); p == nil || p.Summary != "Claude Desktop skipped its last run because of something_new" {
		t.Errorf("unknown reasons should still be reported, got %+v", p)
	}
}

// missedEnv is noon on Sunday Nov 8 with Claude Desktop open since 8:00 and
// the Mac awake since 7:00.
func missedEnv() Env {
	env := testEnv(nil)
	env.Running = func(name string) bool { return name == "Claude" }
	env.Started = func(string) time.Time { return env.Now.Add(-4 * time.Hour) }
	env.Awake = env.Now.Add(-5 * time.Hour)
	return env
}

func TestMissedRun(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	at := func(d, h, m int) time.Time { return time.Date(2026, 11, d, h, m, 0, 0, ny) }
	created := at(1, 10, 0)
	claude := func(expr string, last time.Time) job.Job {
		return job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Host: "Claude Desktop",
			Expr: expr, LastRun: last, Created: created}
	}

	tests := []struct {
		name string
		job  job.Job
		env  func(*Env)
		want string
	}{
		{name: "ran today", job: claude("0 9 * * *", at(8, 9, 3))},
		{
			name: "missed today",
			job:  claude("0 9 * * *", at(7, 9, 1)),
			want: "Claude Desktop didn't run it today at 09:00, and its last run was Nov 7",
		},
		{
			name: "missed several",
			job:  claude("0 9 * * *", at(6, 9, 2)),
			want: "Claude Desktop missed 2 runs, the latest today at 09:00, and its last run was Nov 6",
		},
		{
			name: "missed yesterday",
			job:  claude("0 9 * * 6", time.Date(2026, 10, 31, 9, 0, 0, 0, ny)),
			want: "Claude Desktop didn't run it yesterday at 09:00, and its last run was Oct 31",
		},
		{
			name: "never ran",
			job:  job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Expr: "0 9 * * *", Created: at(5, 10, 0)},
			want: "Claude Desktop missed 3 runs, the latest today at 09:00, and it has never run",
		},
		{name: "due within the random delay", job: claude("0 * * * *", at(8, 11, 2))},
		{name: "created after it was due", job: job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Expr: "0 9 * * *", Created: at(8, 10, 0)}},
		{name: "no dates to go by", job: job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Expr: "0 9 * * *"}},
		{
			name: "app closed",
			job:  claude("0 9 * * *", at(6, 9, 2)),
			env:  func(e *Env) { e.Running = func(string) bool { return false } },
		},
		{
			name: "app just opened, catch-up may be on its way",
			job:  claude("0 9 * * *", at(6, 9, 2)),
			env:  func(e *Env) { e.Started = func(string) time.Time { return e.Now.Add(-5 * time.Minute) } },
		},
		{
			name: "Mac just woke",
			job:  claude("0 9 * * *", at(6, 9, 2)),
			env:  func(e *Env) { e.Awake = e.Now.Add(-2 * time.Minute) },
		},
		{
			name: "already reported as skipped",
			job: func() job.Job {
				j := claude("0 9 * * *", at(6, 9, 2))
				j.Problems = []job.Problem{{Code: "skipped", Severity: job.Fail}}
				return j
			}(),
		},
		{
			name: "disabled",
			job: func() job.Job {
				j := claude("0 9 * * *", at(6, 9, 2))
				j.State = job.StateDisabled
				return j
			}(),
		},
		{
			name: "one-time task that didn't run",
			job:  job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Once: at(8, 9, 0), Created: at(7, 15, 0)},
			want: "Claude Desktop didn't run it today at 09:00, and it has never run",
		},
		{name: "one-time task that ran", job: job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Once: at(8, 9, 0), Created: at(7, 15, 0), LastRun: at(8, 9, 1)}},
		{name: "one-time task still to come", job: job.Job{Source: job.Claude, Kind: job.Scheduled, State: job.StateIdle, Once: at(8, 18, 0), Created: at(7, 15, 0)}},
		{name: "not a Claude task", job: job.Job{Source: job.Hermes, Kind: job.Scheduled, State: job.StateIdle, Expr: "0 9 * * *", LastRun: at(6, 9, 0)}},
	}
	for _, tt := range tests {
		env := missedEnv()
		if tt.env != nil {
			tt.env(&env)
		}
		p := missedRun(&tt.job, env)
		switch {
		case tt.want == "" && p != nil:
			t.Errorf("%s: got %q, want no problem", tt.name, p.Summary)
		case tt.want != "" && p == nil:
			t.Errorf("%s: got no problem, want %q", tt.name, tt.want)
		case p != nil && (p.Summary != tt.want || p.Severity != job.Warn):
			t.Errorf("%s:\n got  %q\n want %q", tt.name, p.Summary, tt.want)
		}
	}
}
