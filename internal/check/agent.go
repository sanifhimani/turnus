package check

import (
	"fmt"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/schedule"
)

// claudeSkips explains why Claude Desktop skipped a run, and how to fix it.
// The reasons are the values the app records in recordedSkips.
var claudeSkips = map[string]struct{ why, fix string }{
	"folder_missing":        {"its folder no longer exists", "Choose a new folder for the task in Claude Desktop"},
	"folder_not_trusted":    {"its folder isn't trusted", "Trust the folder in Claude Desktop"},
	"start_failed":          {"the task failed to start", "Run the task by hand in Claude Desktop to see the error"},
	"sign_in_required":      {"you were signed out", "Sign in to Claude Desktop"},
	"host_unavailable":      {"the computer it runs on was unavailable", "Check that the computer is on and Claude Desktop is open there"},
	"host_not_allowed":      {"it isn't allowed to run on this computer", "Check the task's settings in Claude Desktop"},
	"connection_removed":    {"a connection it uses was removed", "Reconnect the service in Claude Desktop"},
	"per_task_limit":        {"it reached its usage limit", "Check your plan's usage in Claude Desktop"},
	"global_limit":          {"you reached your usage limit", "Check your plan's usage in Claude Desktop"},
	"not_available":         {"scheduled tasks weren't available", "Update Claude Desktop"},
	"not_run_before_update": {"Claude Desktop was updating", "No action needed if the next run works"},
}

// agentSchedule flags a Claude or Hermes task whose cron expression doesn't
// parse. Neither app can run it, and without a next run nothing else about
// the task would look wrong.
func agentSchedule(j *job.Job, env Env) *job.Problem {
	if j.Err != "" || j.Expr == "" || j.State == job.StateDisabled {
		return nil
	}
	if j.Source != job.Claude && j.Source != job.Hermes {
		return nil
	}
	_, err := schedule.ParseCron(j.Expr)
	if err == nil {
		return nil
	}
	if j.Source == job.Claude {
		return &job.Problem{
			Code:     "invalid-schedule",
			Severity: job.Fail,
			Summary:  "Claude Desktop can't run this task because its schedule is invalid (" + err.Error() + ")",
			Fix:      "Edit the task's schedule in Claude Desktop",
		}
	}
	return &job.Problem{
		Code:     "invalid-schedule",
		Severity: job.Fail,
		Summary:  "Hermes can't run this job because its schedule is invalid (" + err.Error() + ")",
		Fix:      "Fix the job's schedule in Hermes",
	}
}

// lastResult reports runs that an agent scheduler recorded as skipped or
// failed. These schedulers record outcomes in words, not exit codes.
func lastResult(j *job.Job, env Env) *job.Problem {
	if j.LastResult == "" || j.State == job.StateDisabled {
		return nil
	}
	switch j.Source {
	case job.Claude:
		s, ok := claudeSkips[j.LastResult]
		if !ok {
			s.why, s.fix = "of "+j.LastResult, "Open the task in Claude Desktop to see why"
		}
		return &job.Problem{
			Code:     "skipped",
			Severity: job.Fail,
			Summary:  "Claude Desktop skipped its last run because " + s.why,
			Fix:      s.fix,
		}
	case job.Hermes:
		fix := "Run hermes cron list to see the error"
		if j.LogPath != "" {
			fix = "Read its output in " + env.tilde(j.LogPath)
		}
		switch j.LastResult {
		case "error":
			return &job.Problem{Code: "failed", Severity: job.Fail, Summary: "Its last run failed", Fix: fix}
		case "blocked_config":
			return &job.Problem{Code: "failed", Severity: job.Fail,
				Summary: "Hermes blocked its last run because of a problem with its settings", Fix: "Check the job's settings in Hermes"}
		case "delivery_failed":
			return &job.Problem{Code: "undelivered", Severity: job.Warn,
				Summary: "Its last run finished, but Hermes couldn't deliver the result", Fix: fix}
		}
	}
	return nil
}

// hostRequired notes jobs that only fire while an app or service runs, and
// warns when Claude Desktop, the one turnus can detect reliably, is closed.
func hostRequired(j *job.Job, env Env) *job.Problem {
	if j.Host == "" || j.State == job.StateDisabled || j.Err != "" {
		return nil
	}
	if j.Source == job.Claude && j.Next.IsZero() {
		return nil // manual and finished one-time tasks don't wait on the app
	}
	if j.Source == job.Claude {
		if env.Running != nil && !env.Running("Claude") {
			return &job.Problem{
				Code:     "host-closed",
				Severity: job.Warn,
				Summary:  "Claude Desktop isn't open, so this task won't run until you open it",
				Fix:      "Open Claude Desktop and keep it open",
			}
		}
		return &job.Problem{
			Code:     "needs-host",
			Severity: job.Info,
			Summary:  "Runs only while Claude Desktop is open and the Mac is awake",
		}
	}
	return &job.Problem{
		Code:     "needs-host",
		Severity: job.Info,
		Summary:  "Runs only while " + j.Host + " is running",
	}
}

const (
	// missedGrace allows for the few minutes of random delay Claude Desktop
	// adds before each run, and for runs that take a while to record.
	missedGrace = 30 * time.Minute

	// settleTime gives Claude Desktop time to run its catch-up after it
	// opens or the Mac wakes, before a missed run is reported.
	settleTime = 10 * time.Minute
)

// missedRun flags Claude Desktop tasks that should have run and didn't,
// though the app was open and the Mac awake. When the app opens or the Mac
// wakes, Claude Desktop runs one catch-up for the latest missed time, so a
// run still missing after that is one the scheduler dropped.
func missedRun(j *job.Job, env Env) *job.Problem {
	if j.Source != job.Claude || j.Err != "" || j.State == job.StateDisabled || hasCode(j, "skipped") {
		return nil
	}
	if env.Running == nil || !env.Running("Claude") {
		return nil // hostRequired explains that the app is closed
	}
	// Until the app and the Mac have been up a while, a catch-up run may
	// still be on its way.
	up := env.Awake
	if env.Started != nil {
		up = latest(up, env.Started("Claude"))
	}
	if up.IsZero() || env.Now.Sub(up) < settleTime {
		return nil
	}

	last := latest(j.LastRun, j.Created)
	cutoff := env.Now.Add(-missedGrace)
	var due time.Time
	n := 0
	switch {
	case j.Expr != "":
		c, err := schedule.ParseCron(j.Expr)
		if err != nil {
			return nil // agentSchedule reports the invalid schedule
		}
		for t := last.In(env.Loc); n < countLimit; n++ {
			next := c.Next(t)
			if next.IsZero() || next.After(cutoff) {
				break
			}
			due, t = next, next
		}
	case !j.Once.IsZero():
		if j.LastRun.IsZero() && j.Once.Before(cutoff) && j.Once.After(j.Created) {
			due, n = j.Once, 1
		}
	}
	if n == 0 || last.IsZero() {
		return nil
	}

	since := "it has never run"
	if !j.LastRun.IsZero() {
		since = "its last run was " + day(j.LastRun, env)
	}
	summary := fmt.Sprintf("Claude Desktop didn't run it %s, and %s", when(due, env), since)
	if n > 1 {
		summary = fmt.Sprintf("Claude Desktop missed %d runs, the latest %s, and %s", n, when(due, env), since)
	}
	return &job.Problem{
		Code:     "missed-run",
		Severity: job.Warn,
		Summary:  summary,
		Fix:      "Quit and reopen Claude Desktop, which restarts its scheduler and runs the missed task once",
	}
}

// when describes a recent time for a sentence: "today at 09:00",
// "yesterday at 09:00", or "on Nov 10 at 09:00".
func when(t time.Time, env Env) string {
	t, now := t.In(env.Loc), env.Now.In(env.Loc)
	clock := t.Format("15:04")
	y, m, d := now.Date()
	switch ty, tm, td := t.Date(); {
	case ty == y && tm == m && td == d:
		return "today at " + clock
	case ty == y && tm == m && td == d-1:
		return "yesterday at " + clock
	}
	return "on " + day(t, env) + " at " + clock
}
