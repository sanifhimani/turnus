package crontab

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/schedule"
)

// Periodic directories run by run-parts or anacron on most Linux systems.
var periodic = []struct{ dir, expr, schedule string }{
	{"/etc/cron.hourly", "@hourly", "hourly"},
	{"/etc/cron.daily", "@daily", "daily"},
	{"/etc/cron.weekly", "@weekly", "weekly"},
	{"/etc/cron.monthly", "@monthly", "monthly"},
}

// Discover returns jobs from the current user's crontab and the system
// crontabs. Missing files are not errors.
func Discover(ctx context.Context, now time.Time) ([]job.Job, error) {
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

	for _, p := range periodic {
		jobs = append(jobs, discoverPeriodic(p.dir, p.expr, p.schedule, now)...)
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
	if err != nil {
		return nil
	}
	defer f.Close()

	entries, err := Parse(f, true)
	if err != nil {
		return nil
	}
	jobs := make([]job.Job, 0, len(entries))
	for _, e := range entries {
		jobs = append(jobs, toJob(e, path, job.System, now))
	}
	return jobs
}

func discoverPeriodic(dir, expr, sched string, now time.Time) []job.Job {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	c, _ := schedule.ParseCron(expr)
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
			Expr:     expr,
			Next:     c.Next(now),
			State:    job.StateIdle,
			File:     path,
		})
	}
	return jobs
}

func toJob(e Entry, file string, scope job.Scope, now time.Time) job.Job {
	name, program := commandName(e.Command)
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
		LogPath:  logPath(e.Command),
	}

	c, err := schedule.ParseCron(e.Expr)
	if err != nil {
		j.State = job.StateUnknown
		return j
	}
	if d := c.Describe(); d != "" {
		j.Schedule = d
	}
	j.Next = c.Next(now.In(location(e.Env)))
	return j
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
