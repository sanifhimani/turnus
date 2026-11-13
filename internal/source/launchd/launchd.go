// Package launchd finds jobs defined in launchd property lists on macOS.
package launchd

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

type location struct {
	dir     string
	daemon  bool
	builtin bool
}

func locations(home string) []location {
	return []location{
		{dir: filepath.Join(home, "Library/LaunchAgents")},
		{dir: "/Library/LaunchAgents"},
		{dir: "/Library/LaunchDaemons", daemon: true},
		{dir: "/System/Library/LaunchAgents", builtin: true},
		{dir: "/System/Library/LaunchDaemons", daemon: true, builtin: true},
	}
}

// Discover returns launchd jobs. Jobs that ship with macOS are included only
// when builtin is true.
func Discover(ctx context.Context, now time.Time, builtin bool) ([]job.Job, error) {
	if runtime.GOOS != "darwin" {
		return nil, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	var (
		wg           sync.WaitGroup
		gui, system  domain
		guiDomain    = "gui/" + strconv.Itoa(os.Getuid())
		guiErr, sErr error
	)
	wg.Go(func() { gui, guiErr = printDomain(ctx, guiDomain) })
	wg.Go(func() { system, sErr = printDomain(ctx, "system") })

	jobs := readAll(home, builtin, now)

	wg.Wait()
	for i := range jobs {
		j := &jobs[i]
		d, ok := gui, guiErr == nil
		if j.Scope == job.System {
			d, ok = system, sErr == nil
		}
		if ok {
			applyStatus(j, d)
		}
	}
	return jobs, nil
}

type file struct {
	path string
	loc  location
}

// readAll parses every plist, spread across CPUs. With builtin jobs there
// are close to a thousand files.
func readAll(home string, builtin bool, now time.Time) []job.Job {
	var files []file
	for _, loc := range locations(home) {
		if loc.builtin && !builtin {
			continue
		}
		paths, _ := filepath.Glob(filepath.Join(loc.dir, "*.plist"))
		for _, p := range paths {
			files = append(files, file{p, loc})
		}
	}

	jobs := make([]job.Job, len(files))
	workers := min(runtime.GOMAXPROCS(0), len(files))
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := w; i < len(files); i += workers {
				jobs[i] = readJob(files[i].path, files[i].loc, now)
			}
		})
	}
	wg.Wait()
	return jobs
}

func readJob(path string, loc location, now time.Time) job.Job {
	j := job.Job{
		ID:      "launchd:" + path,
		Name:    strings.TrimSuffix(filepath.Base(path), ".plist"),
		Source:  job.Launchd,
		Scope:   job.User,
		Kind:    job.Service,
		Builtin: loc.builtin,
		State:   job.StateUnknown,
		File:    path,
	}
	if loc.daemon {
		j.Scope = job.System
	}

	s, err := readSpec(path)
	if err != nil {
		j.Schedule = "unreadable"
		j.Err = err.Error()
		if errors.Is(err, fs.ErrPermission) {
			j.Err, j.Unreadable = "permission denied", true
		}
		return j
	}
	if s.Label != "" {
		j.Name = s.Label
	}
	j.Command = s.command()
	j.Program = s.program()
	// Without a Program key, launchd finds a bare first argument by
	// searching a fixed PATH, ignoring the job's EnvironmentVariables.
	if s.Program == "" && j.Program != "" && !strings.Contains(j.Program, "/") {
		j.Lookups = []string{j.Program}
	}
	j.LogPath = s.StandardOutPath
	if j.LogPath == "" {
		j.LogPath = s.StandardErrorPath
	}
	if s.Disabled {
		j.State = job.StateDisabled
	}

	t := s.trigger(loc.daemon, now)
	j.Schedule, j.Expr, j.Next = t.desc, t.expr, t.next
	if t.scheduled {
		j.Kind = job.Scheduled
	}
	return j
}

// applyStatus fills in the job's state from launchctl. An override in the
// disabled list wins over the plist's own Disabled key.
func applyStatus(j *job.Job, d domain) {
	if disabled, ok := d.disabled[j.Name]; ok {
		if disabled {
			j.State = job.StateDisabled
			return
		}
		if j.State == job.StateDisabled {
			j.State = job.StateUnknown
		}
	}
	if j.State == job.StateDisabled {
		return
	}

	svc, loaded := d.services[j.Name]
	switch {
	case !loaded:
		j.State = job.StateUnloaded
	case svc.pid > 0:
		j.State = job.StateRunning
		j.PID = svc.pid
	default:
		j.State = job.StateIdle
	}
	j.LastExit = svc.lastExit
}
