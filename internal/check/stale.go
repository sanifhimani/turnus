package check

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

const (
	// staleRuns is how many due runs must pass without a log write before
	// turnus says something. One or two can be a sleeping laptop or a job
	// that's still running.
	staleRuns = 3

	// staleAfter keeps jobs that run every minute from being flagged after
	// a few quiet minutes.
	staleAfter = time.Hour

	// countLimit caps how many due runs are counted for very frequent jobs.
	countLimit = 10000
)

// staleLog flags scheduled jobs whose log hasn't changed through several
// runs they were due for. cron keeps no history, so a log that stops
// changing is often the only sign a job stopped running.
//
// A log that was never created is strong evidence: the shell or launchd
// creates it when the job starts, even if the job prints nothing. A log that
// exists but stopped changing is weaker, since a job that prints only on
// errors leaves its log alone when it works.
func staleLog(j *job.Job, env Env) *job.Problem {
	if j.Kind != job.Scheduled || j.Err != "" || j.LogPath == "" || hasFail(j) {
		return nil
	}
	switch {
	case j.Source == job.Launchd && !loaded(j):
		return nil
	case j.Source != job.Cron && j.Source != job.Launchd:
		return nil
	case strings.HasPrefix(j.LogPath, "/dev/"):
		return nil
	}

	saved := savedAt(j, env)
	start := latest(saved, env.Booted)

	var summary, fix string
	fi, err := env.Stat(j.LogPath)
	switch {
	case err == nil && fi.Mode().IsRegular():
		// An empty log means the job has never printed, so its silence
		// says nothing about whether it runs.
		if fi.Size() == 0 {
			return nil
		}
		start = latest(start, fi.ModTime())
		n, ok := overdue(j, start, env)
		if !ok {
			return nil
		}
		summary = fmt.Sprintf("Nothing written to its log since %s, though it was due to run %s", day(fi.ModTime(), env), n)
		fix = "If it writes only on errors, ignore this. Otherwise run it by hand to see why it isn't running"
	case errors.Is(err, fs.ErrNotExist):
		// Without knowing when the job was set up, the runs it missed
		// can't be counted.
		if saved.IsZero() {
			return nil
		}
		n, ok := overdue(j, start, env)
		if !ok {
			return nil
		}
		summary = fmt.Sprintf("Its log was never created, though it was due to run %s since %s", n, day(start, env))
		fix = "Run it by hand to see why it isn't running"
	default:
		return nil
	}

	if env.GOOS == "darwin" {
		switch {
		case j.Source == job.Cron:
			fix += ". cron also skips runs while the Mac is asleep"
		case strings.HasPrefix(j.Expr, "StartInterval "):
			fix += ". StartInterval also skips runs while the Mac is asleep"
		}
	}
	return &job.Problem{Code: "stale-log", Severity: job.Warn, Summary: summary, Fix: fix}
}

// overdue counts the runs due after start and reports whether there are
// enough to flag.
func overdue(j *job.Job, start time.Time, env Env) (string, bool) {
	if !j.LastRun.IsZero() && j.LastRun.After(start) {
		return "", false
	}
	if env.Now.Sub(start) < staleAfter {
		return "", false
	}
	n := due(j, start, env)
	if n < staleRuns {
		return "", false
	}
	if n >= countLimit {
		return fmt.Sprintf("more than %d times", countLimit), true
	}
	return fmt.Sprintf("%d times", n), true
}

// due counts the runs a job's schedule called for after start, up to now.
func due(j *job.Job, start time.Time, env Env) int {
	var secs int
	if _, err := fmt.Sscanf(j.Expr, "StartInterval %d", &secs); err == nil {
		if secs <= 0 {
			return 0
		}
		return min(int(env.Now.Sub(start)/(time.Duration(secs)*time.Second)), countLimit)
	}
	n := 0
	for _, c := range crons(j) {
		for t := start.In(env.Loc); n < countLimit; n++ {
			if t = c.Next(t); t.IsZero() || t.After(env.Now) {
				break
			}
		}
	}
	return min(n, countLimit)
}

// savedAt returns when the job's definition last changed, or the zero time
// if turnus can't tell. A user's crontab can't be read without root, but
// its folder can, and crontab replaces the file there on every save.
func savedAt(j *job.Job, env Env) time.Time {
	if fi, err := env.Stat(j.File); err == nil {
		return fi.ModTime()
	}
	if j.Source == job.Cron {
		if fi, err := env.Stat(filepath.Dir(j.File)); err == nil {
			return fi.ModTime()
		}
	}
	return time.Time{}
}

func latest(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// day formats a date for a sentence, adding the year when it isn't this one.
func day(t time.Time, env Env) string {
	t = t.In(env.Loc)
	if t.Year() == env.Now.Year() {
		return t.Format("Jan 2")
	}
	return t.Format("Jan 2, 2006")
}
