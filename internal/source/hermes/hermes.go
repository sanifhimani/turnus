// Package hermes finds jobs scheduled by the Hermes agent from Nous Research.
// Its gateway service stores them in ~/.hermes/cron/jobs.json, as described
// at https://hermes-agent.nousresearch.com/docs/developer-guide/cron-internals.
package hermes

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/schedule"
	"github.com/sanifhimani/turnus/internal/source"
)

// Host is what must be running for Hermes jobs to fire.
const Host = "the Hermes gateway"

type entry struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Prompt   string `json:"prompt"`
	Enabled  *bool  `json:"enabled"`
	State    string `json:"state"`
	PausedAt any    `json:"paused_at"`
	Schedule struct {
		Kind    string `json:"kind"`
		Expr    string `json:"expr"`
		Minutes int    `json:"minutes"`
		Display string `json:"display"`
	} `json:"schedule"`
	ScheduleDisplay string      `json:"schedule_display"`
	Timezone        string      `json:"timezone"`
	NextRunAt       source.Time `json:"next_run_at"`
	LastRunAt       source.Time `json:"last_run_at"`
	LastStatus      string      `json:"last_status"`
}

// Discover returns the jobs in the Hermes store. HERMES_HOME moves the store
// from ~/.hermes.
func Discover(now time.Time) ([]job.Job, error) {
	dir := os.Getenv("HERMES_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(home, ".hermes")
	}
	return read(filepath.Join(dir, "cron", "jobs.json"), now), nil
}

func read(path string, now time.Time) []job.Job {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return []job.Job{broken(path, err.Error())}
	}

	// The store is an object with a jobs list. Older versions wrote a bare list.
	var store struct {
		Jobs []entry `json:"jobs"`
	}
	if err := json.Unmarshal(data, &store); err != nil {
		if err := json.Unmarshal(data, &store.Jobs); err != nil {
			return []job.Job{broken(path, "its format isn't one turnus knows")}
		}
	}

	jobs := make([]job.Job, 0, len(store.Jobs))
	for _, e := range store.Jobs {
		jobs = append(jobs, toJob(e, path, now))
	}
	return jobs
}

func toJob(e entry, path string, now time.Time) job.Job {
	j := job.Job{
		ID:         "hermes:" + e.ID,
		Name:       e.Name,
		Source:     job.Hermes,
		Scope:      job.User,
		Kind:       job.Scheduled,
		Command:    source.Summary(e.Prompt),
		State:      job.StateIdle,
		File:       path,
		Host:       Host,
		LastRun:    e.LastRunAt.Time,
		LastResult: e.LastStatus,
		Next:       e.NextRunAt.Time,
	}
	if j.Name == "" {
		j.Name = e.ID
	}

	switch {
	case e.Enabled != nil && !*e.Enabled, e.PausedAt != nil, e.State == "paused":
		j.State = job.StateDisabled
	case e.State == "running":
		j.State = job.StateRunning
	}

	j.Schedule = e.Schedule.Display
	if j.Schedule == "" {
		j.Schedule = e.ScheduleDisplay
	}
	switch e.Schedule.Kind {
	case "cron":
		j.Expr = e.Schedule.Expr
		if c, err := schedule.ParseCron(e.Schedule.Expr); err == nil {
			if d := c.Describe(); d != "" {
				j.Schedule = d
			}
			if j.Next.IsZero() {
				j.Next = c.Next(now.In(location(e.Timezone)))
			}
		}
	case "interval":
		if e.Schedule.Minutes > 0 {
			j.Schedule = schedule.Every(time.Duration(e.Schedule.Minutes) * time.Minute)
		}
	}
	if j.Schedule == "" {
		j.Schedule = e.Schedule.Expr
	}
	if j.State == job.StateDisabled || j.Next.Before(now) {
		j.Next = time.Time{}
	}

	j.LogPath = latestOutput(filepath.Join(filepath.Dir(path), "output", e.ID))
	return j
}

// latestOutput returns the newest file Hermes wrote for a job. Hermes names
// them by timestamp, so the last name sorts newest.
func latestOutput(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for i := len(entries) - 1; i >= 0; i-- {
		if !entries[i].IsDir() {
			return filepath.Join(dir, entries[i].Name())
		}
	}
	return ""
}

func location(tz string) *time.Location {
	if tz != "" {
		if loc, err := time.LoadLocation(tz); err == nil {
			return loc
		}
	}
	return time.Local
}

// broken stands in for a store turnus couldn't read.
func broken(path, reason string) job.Job {
	return job.Job{
		ID:       "hermes:" + path,
		Name:     "hermes jobs",
		Source:   job.Hermes,
		Scope:    job.User,
		Kind:     job.Scheduled,
		Schedule: "unreadable",
		State:    job.StateUnknown,
		File:     path,
		Err:      reason,
	}
}
