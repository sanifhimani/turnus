// Package claude finds the scheduled tasks Claude Desktop runs on this Mac.
//
// The app documents only each task's prompt file. The schedule, state and
// skipped runs live in scheduled-tasks.json files under its support folder,
// in a format turnus reads from the app's own code. It isn't a published
// format, so every field is optional and a file that won't parse is reported
// rather than guessed at.
package claude

import (
	"bufio"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/schedule"
	"github.com/sanifhimani/turnus/internal/source"
)

// Host is what must be running for Claude tasks to fire.
const Host = "Claude Desktop"

type store struct {
	Tasks []task            `json:"scheduledTasks"`
	Skips map[string][]skip `json:"recordedSkips"`
}

type task struct {
	ID             string      `json:"id"`
	DisplayName    string      `json:"displayName"`
	CronExpression string      `json:"cronExpression"`
	FireAt         source.Time `json:"fireAt"`
	Enabled        *bool       `json:"enabled"`
	FilePath       string      `json:"filePath"`
	LastRunAt      source.Time `json:"lastRunAt"`
}

// skip is a run the app didn't make, with its reason, such as
// "folder_missing" or "sign_in_required".
type skip struct {
	At     source.Time `json:"at"`
	Reason string      `json:"reason"`
}

// Discover returns tasks from Claude Desktop's Code tab and from Cowork,
// which keep separate stores per account.
func Discover(now time.Time) ([]job.Job, error) {
	if runtime.GOOS != "darwin" {
		return nil, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	base := filepath.Join(home, "Library", "Application Support", "Claude")
	var paths []string
	for _, area := range []string{"claude-code-sessions", "local-agent-mode-sessions"} {
		found, _ := filepath.Glob(filepath.Join(base, area, "*", "*", "scheduled-tasks.json"))
		paths = append(paths, found...)
	}

	var jobs []job.Job
	for _, p := range paths {
		jobs = append(jobs, read(p, now)...)
	}
	return jobs, nil
}

func read(path string, now time.Time) []job.Job {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	var s store
	if err == nil {
		err = json.Unmarshal(data, &s)
	}
	if err != nil {
		return []job.Job{{
			ID:       "claude:" + path,
			Name:     "claude tasks",
			Source:   job.Claude,
			Scope:    job.User,
			Kind:     job.Scheduled,
			Schedule: "unreadable",
			State:    job.StateUnknown,
			File:     path,
			Err:      "its format isn't one turnus knows",
		}}
	}

	jobs := make([]job.Job, 0, len(s.Tasks))
	for _, t := range s.Tasks {
		jobs = append(jobs, toJob(t, s.Skips[t.ID], path, now))
	}
	return jobs
}

func toJob(t task, skips []skip, store string, now time.Time) job.Job {
	j := job.Job{
		ID:      "claude:" + t.ID,
		Name:    t.DisplayName,
		Source:  job.Claude,
		Scope:   job.User,
		Kind:    job.Scheduled,
		Command: describePrompt(t.FilePath),
		State:   job.StateIdle,
		File:    store,
		Host:    Host,
		LastRun: t.LastRunAt.Time,
	}
	if j.Name == "" {
		j.Name = t.ID
	}
	if t.FilePath != "" {
		j.File = t.FilePath
	}
	if t.Enabled != nil && !*t.Enabled {
		j.State = job.StateDisabled
	}

	switch {
	case t.CronExpression != "":
		j.Schedule, j.Expr = t.CronExpression, t.CronExpression
		if c, err := schedule.ParseCron(t.CronExpression); err == nil {
			if d := c.Describe(); d != "" {
				j.Schedule = d
			}
			j.Next = c.Next(now)
		}
	case !t.FireAt.IsZero():
		j.Schedule = "once at " + t.FireAt.Local().Format("Jan 2 15:04")
		if t.LastRunAt.IsZero() && t.FireAt.After(now) {
			j.Next = t.FireAt.Time
		}
	default:
		j.Schedule = "manual"
	}
	if j.State == job.StateDisabled {
		j.Next = time.Time{}
	}

	// A skip newer than the last run is why the task didn't run last time.
	if len(skips) > 0 {
		latest := slices.MaxFunc(skips, func(a, b skip) int { return a.At.Compare(b.At.Time) })
		if latest.At.After(j.LastRun) {
			j.LastResult = latest.Reason
		}
	}
	return j
}

// describePrompt reads the description from a task's SKILL.md front matter,
// or the start of the prompt when there's none.
func describePrompt(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	inFront := false
	var body []string
	for i := 0; sc.Scan() && i < 200; i++ {
		line := sc.Text()
		switch {
		case i == 0 && line == "---":
			inFront = true
		case inFront && line == "---":
			inFront = false
		case inFront:
			if v, ok := strings.CutPrefix(line, "description:"); ok {
				return source.Summary(strings.Trim(strings.TrimSpace(v), `"'`))
			}
		case strings.TrimSpace(line) != "":
			body = append(body, line)
		}
		if len(body) > 0 {
			break
		}
	}
	return source.Summary(strings.Join(body, "\n"))
}
