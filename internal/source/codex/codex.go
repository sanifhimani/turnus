// Package codex finds the scheduled tasks Codex runs in the ChatGPT desktop
// app. The app keeps each one in ~/.codex/automations/<id>/automation.toml
// and runs local ones only while it is open. The format is not documented,
// so fields are read leniently.
package codex

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/schedule"
	"github.com/sanifhimani/turnus/internal/source"
)

// Host is what must be running for Codex scheduled tasks to fire.
const Host = "the ChatGPT desktop app"

type automation struct {
	ID          string `toml:"id"`
	Name        string `toml:"name"`
	Prompt      string `toml:"prompt"`
	Status      string `toml:"status"`
	RRule       string `toml:"rrule"`
	Environment string `toml:"execution_environment"`
}

// Discover returns local scheduled tasks. CODEX_HOME moves them from
// ~/.codex. Tasks that run in the cloud are left out.
func Discover(now time.Time) ([]job.Job, error) {
	dir := os.Getenv("CODEX_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".codex")
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "automations", "*", "automation.toml"))

	var jobs []job.Job
	for _, p := range paths {
		if j, ok := read(p, now); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func read(path string, now time.Time) (job.Job, bool) {
	var a automation
	_, err := toml.DecodeFile(path, &a)
	if errors.Is(err, fs.ErrNotExist) {
		return job.Job{}, false
	}
	if err != nil {
		return job.Job{
			ID:       "codex:" + path,
			Name:     filepath.Base(filepath.Dir(path)),
			Source:   job.Codex,
			Scope:    job.User,
			Kind:     job.Scheduled,
			Schedule: "unreadable",
			State:    job.StateUnknown,
			File:     path,
			Err:      "its format isn't one turnus knows",
		}, true
	}
	if a.Environment != "" && a.Environment != "local" {
		return job.Job{}, false
	}

	j := job.Job{
		ID:       "codex:" + a.ID,
		Name:     a.Name,
		Source:   job.Codex,
		Scope:    job.User,
		Kind:     job.Scheduled,
		Command:  source.Summary(a.Prompt),
		Schedule: a.RRule,
		State:    job.StateIdle,
		File:     path,
		Host:     Host,
	}
	if j.Name == "" {
		j.Name = filepath.Base(filepath.Dir(path))
	}
	if a.Status != "" && a.Status != "ACTIVE" {
		j.State = job.StateDisabled
	}
	if expr, ok := schedule.CronFromRRule(a.RRule); ok {
		j.Expr = expr
		c, _ := schedule.ParseCron(expr)
		if d := c.Describe(); d != "" {
			j.Schedule = d
		}
		if j.State != job.StateDisabled {
			j.Next = c.Next(now)
		}
	}
	return j, true
}
