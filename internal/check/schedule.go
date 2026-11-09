package check

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/schedule"
)

// crons parses the job's schedule as cron expressions. launchd calendar
// jobs and systemd timers may have several, joined with ", ". It returns nil
// if any can't be read as cron.
func crons(j *job.Job) []*schedule.Cron {
	if j.Kind != job.Scheduled || j.Expr == "" {
		return nil
	}
	var out []*schedule.Cron
	for e := range strings.SplitSeq(j.Expr, ", ") {
		if j.Source == job.Systemd {
			var ok bool
			if e, ok = schedule.CronFromCalendar(e); !ok {
				return nil
			}
		}
		c, err := schedule.ParseCron(e)
		if err != nil {
			return nil
		}
		out = append(out, c)
	}
	return out
}

func neverRuns(j *job.Job, env Env) *job.Problem {
	cs := crons(j)
	if len(cs) == 0 {
		return nil
	}
	for _, c := range cs {
		if c.Reboot || !c.Next(env.Now).IsZero() {
			return nil
		}
	}
	return &job.Problem{
		Code:     "never-runs",
		Severity: job.Fail,
		Summary:  "Schedule never matches a real date",
		Fix:      "Check the day and month in " + j.Expr,
	}
}

// missedWhileAsleep covers schedulers that drop runs while the machine is
// asleep or off: cron and StartInterval on macOS, and systemd timers without
// Persistent=true.
func missedWhileAsleep(j *job.Job, env Env) *job.Problem {
	if j.Kind != job.Scheduled {
		return nil
	}
	if j.Source == job.Systemd {
		if j.Persistent || !loaded(j) || !infrequent(crons(j), env) {
			return nil
		}
		return &job.Problem{
			Code:     "missed-while-off",
			Severity: job.Info,
			Summary:  "Runs missed while the machine is off are skipped",
			Fix:      "Add Persistent=true under [Timer] in " + env.tilde(j.File),
		}
	}
	if env.GOOS != "darwin" {
		return nil
	}
	switch j.Source {
	case job.Cron:
		if !infrequent(crons(j), env) {
			return nil
		}
		return &job.Problem{
			Code:     "missed-while-asleep",
			Severity: job.Info,
			Summary:  "cron skips runs while the Mac is asleep",
			Fix:      "Use a launchd agent with StartCalendarInterval, which runs missed jobs on wake",
		}
	case job.Launchd:
		var secs int
		if _, err := fmt.Sscanf(j.Expr, "StartInterval %d", &secs); err != nil || secs < 3600 {
			return nil
		}
		return &job.Problem{
			Code:     "missed-while-asleep",
			Severity: job.Info,
			Summary:  "StartInterval skips runs while the Mac is asleep",
			Fix:      "Switch to StartCalendarInterval, which runs missed jobs on wake",
		}
	}
	return nil
}

// infrequent reports whether a schedule runs only a few times a day. Jobs
// that run many times a day lose little to a skipped run.
func infrequent(cs []*schedule.Cron, env Env) bool {
	if len(cs) == 0 {
		return false
	}
	for _, c := range cs {
		if c.Reboot || c.Next(env.Now).IsZero() {
			return false
		}
		if hours := c.Hours(); len(hours) == 0 || len(hours) > 4 {
			return false
		}
	}
	return true
}

// daylightSaving flags jobs pinned to 1 or 2 a.m., when most time zones
// change their clocks. Those runs can be skipped or happen twice.
func daylightSaving(j *job.Job, env Env) *job.Problem {
	if !observesDST(env.Loc, env.Now) {
		return nil
	}
	for _, c := range crons(j) {
		hours := c.Hours()
		if len(hours) == 0 || len(hours) > 4 {
			continue
		}
		if slices.Contains(hours, 1) || slices.Contains(hours, 2) {
			return &job.Problem{
				Code:     "daylight-saving",
				Severity: job.Info,
				Summary:  "Runs between 1 and 3 a.m., when daylight saving can skip it or run it twice",
				Fix:      "Move it outside 01:00 to 02:59",
			}
		}
	}
	return nil
}

func observesDST(loc *time.Location, now time.Time) bool {
	if loc == nil {
		return false
	}
	_, jan := time.Date(now.Year(), 1, 1, 12, 0, 0, 0, loc).Zone()
	_, jul := time.Date(now.Year(), 7, 1, 12, 0, 0, 0, loc).Zone()
	return jan != jul
}
