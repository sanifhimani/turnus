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
			name: "codex automation",
			job:  job.Job{Source: job.Codex, Kind: job.Scheduled, State: job.StateIdle, Host: "the ChatGPT app", Next: next},
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
