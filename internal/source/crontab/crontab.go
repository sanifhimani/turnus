package crontab

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/schedule"
)

// Periodic folders, run by run-parts from a crontab line or by anacron on
// most Linux systems.
var periodic = []struct{ dir, schedule string }{
	{"/etc/cron.hourly", "hourly"},
	{"/etc/cron.daily", "daily"},
	{"/etc/cron.weekly", "weekly"},
	{"/etc/cron.monthly", "monthly"},
}

const anacron = "/usr/sbin/anacron"

// Discover returns jobs from the current user's crontab and the system
// crontabs. Missing files are not errors. Jobs installed by OS packages are
// included only when builtin is true.
func Discover(ctx context.Context, now time.Time, builtin bool) ([]job.Job, error) {
	var jobs []job.Job

	userJobs, err := discoverUser(ctx, now)
	if err != nil {
		return nil, err
	}
	jobs = append(jobs, userJobs...)

	jobs = append(jobs, discoverFile("/etc/crontab", now)...)

	files, _ := filepath.Glob("/etc/cron.d/*")
	for _, f := range files {
		// cron skips files with dots in their names, such as editor backups.
		if strings.ContainsAny(filepath.Base(f), ".~") {
			continue
		}
		jobs = append(jobs, discoverFile(f, now)...)
	}

	_, err = os.Stat(anacron)
	hasAnacron := err == nil
	for _, p := range periodic {
		r := findRunner(jobs, p.dir, hasAnacron)
		jobs = append(jobs, discoverPeriodic(p.dir, p.schedule, r, now)...)
	}

	markBuiltin(jobs, packageFiles(dpkgStatus))
	if !builtin {
		jobs = slices.DeleteFunc(jobs, func(j job.Job) bool { return j.Builtin })
	}
	return jobs, nil
}

func discoverUser(ctx context.Context, now time.Time) ([]job.Job, error) {
	if _, err := exec.LookPath("crontab"); err != nil {
		return nil, nil
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "crontab", "-l")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if strings.Contains(stderr.String(), "no crontab") {
			return nil, nil
		}
		return nil, fmt.Errorf("crontab -l: %s", strings.TrimSpace(stderr.String()))
	}

	entries, err := Parse(&stdout, false)
	if err != nil {
		return nil, err
	}
	path := userTabPath()
	jobs := make([]job.Job, 0, len(entries))
	for _, e := range entries {
		jobs = append(jobs, toJob(e, path, job.User, now))
	}
	return jobs, nil
}

func discoverFile(path string, now time.Time) []job.Job {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []job.Job{unreadable(path, err)}
	}
	defer f.Close()

	// A read error part way through still leaves the lines before it.
	entries, err := Parse(f, true)
	if err != nil && len(entries) == 0 {
		return []job.Job{unreadable(path, err)}
	}
	jobs := make([]job.Job, 0, len(entries))
	for _, e := range entries {
		jobs = append(jobs, toJob(e, path, job.System, now))
	}
	return jobs
}

// unreadable stands in for a crontab file turnus couldn't open, so the
// file shows up instead of its jobs silently going missing.
func unreadable(path string, err error) job.Job {
	reason := err.Error()
	if errors.Is(err, fs.ErrPermission) {
		reason = "permission denied"
	}
	return job.Job{
		ID:         "cron:" + path,
		Name:       filepath.Base(path),
		Source:     job.Cron,
		Scope:      job.System,
		Kind:       job.Scheduled,
		Schedule:   "unreadable",
		State:      job.StateUnknown,
		File:       path,
		Err:        reason,
		Unreadable: true,
	}
}

// runner is what runs a periodic folder: a crontab line, anacron, or
// nothing turnus can find.
type runner struct {
	expr    string
	anacron bool
}

// findRunner looks for the crontab line that runs dir with run-parts. On
// Debian the daily, weekly and monthly lines defer to anacron when it is
// installed. Red Hat has no such lines and runs those folders from anacron.
func findRunner(jobs []job.Job, dir string, hasAnacron bool) runner {
	for _, j := range jobs {
		if !strings.Contains(j.Command, "run-parts") || !strings.Contains(j.Command, dir) {
			continue
		}
		if hasAnacron && strings.Contains(j.Command, "anacron") {
			return runner{anacron: true}
		}
		return runner{expr: j.Expr}
	}
	return runner{anacron: hasAnacron}
}

func discoverPeriodic(dir, sched string, r runner, now time.Time) []job.Job {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var next time.Time
	switch {
	case r.anacron:
		sched += " through anacron"
	case r.expr != "":
		if c, err := schedule.ParseCron(r.expr); err == nil {
			if d := c.Describe(); d != "" {
				sched = d
			}
			next = c.Next(now)
		}
	}

	var jobs []job.Job
	for _, de := range entries {
		name := de.Name()
		if de.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		path := filepath.Join(dir, name)
		jobs = append(jobs, job.Job{
			ID:       "cron:" + path,
			Name:     name,
			Source:   job.Cron,
			Scope:    job.System,
			Kind:     job.Scheduled,
			Command:  path,
			Program:  path,
			Schedule: sched,
			Expr:     r.expr,
			Next:     next,
			State:    job.StateIdle,
			File:     path,
		})
	}
	return jobs
}

func toJob(e Entry, file string, scope job.Scope, now time.Time) job.Job {
	if e.Err != "" {
		return job.Job{
			ID:       fmt.Sprintf("cron:%s:%d", file, e.Line),
			Name:     fmt.Sprintf("line %d", e.Line),
			Source:   job.Cron,
			Scope:    scope,
			Kind:     job.Scheduled,
			Command:  e.Command,
			Schedule: "invalid",
			State:    job.StateUnknown,
			File:     file,
			Line:     e.Line,
			Err:      e.Err,
		}
	}

	// cron ends the command at the first unescaped %, so names and log
	// paths come from the part before it.
	runs := beforePercent(e.Command)
	name, program, interpreted := commandName(runs)
	j := job.Job{
		ID:       fmt.Sprintf("cron:%s:%d", file, e.Line),
		Name:     name,
		Source:   job.Cron,
		Scope:    scope,
		Kind:     job.Scheduled,
		Command:  e.Command,
		Program:  program,
		Env:      e.Env,
		Schedule: e.Expr,
		Expr:     e.Expr,
		State:    job.StateIdle,
		File:     file,
		Line:     e.Line,
		LogPath:  logPath(runs),

		Interpreted: interpreted,
		Lookups:     lookups(runs, e.Env),
		SearchPath:  e.Env["PATH"],
	}

	c, err := schedule.ParseCron(e.Expr)
	if err != nil {
		j.State = job.StateUnknown
		j.Err = "its schedule is invalid (" + err.Error() + ")"
		return j
	}
	if d := c.Describe(); d != "" {
		j.Schedule = d
	}
	j.Next = c.Next(now.In(location(e.Env)))
	return j
}

// lookups returns the programs the command runs by bare name, unless the
// crontab picks a shell that may set its own PATH: zsh and fish read the
// user's startup files, and BASH_ENV and ENV name a file the shell reads first.
func lookups(cmd string, env map[string]string) []string {
	if profileShells[filepath.Base(env["SHELL"])] || env["BASH_ENV"] != "" || env["ENV"] != "" {
		return nil
	}
	return pathLookups(cmd)
}

// location honors CRON_TZ, which most cron implementations support.
func location(env map[string]string) *time.Location {
	for _, k := range []string{"CRON_TZ", "TZ"} {
		if tz := env[k]; tz != "" {
			if loc, err := time.LoadLocation(tz); err == nil {
				return loc
			}
		}
	}
	return time.Local
}

func userTabPath() string {
	u, err := user.Current()
	if err != nil {
		return "crontab"
	}
	if runtime.GOOS == "darwin" {
		return "/usr/lib/cron/tabs/" + u.Username
	}
	if _, err := os.Stat("/var/spool/cron/crontabs"); err == nil {
		return "/var/spool/cron/crontabs/" + u.Username
	}
	return "/var/spool/cron/" + u.Username
}
