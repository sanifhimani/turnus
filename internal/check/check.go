// Package check finds problems with jobs: things that are broken, and things
// that will quietly not run when you expect them to.
package check

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

// Env is the machine the checks run against. Tests replace Stat to fake the
// file system.
type Env struct {
	GOOS string
	Home string
	UID  int
	Now  time.Time
	Loc  *time.Location
	Stat func(string) (os.FileInfo, error)

	// HasMail reports whether cron can deliver mail. Without a mail server,
	// cron drops job output.
	HasMail func() bool

	// DebianRunParts is set on Debian and its derivatives, whose run-parts
	// only runs scripts named with letters, digits, - and _.
	DebianRunParts bool

	// Running reports whether a process with this exact name is running.
	Running func(name string) bool

	// CronPath is the PATH cron gives jobs whose crontab doesn't set one.
	CronPath string

	// LookPath finds a program in the PATH turnus runs with, which is
	// usually the user's shell.
	LookPath func(name string) (string, error)

	// Booted is when the machine last started, or the zero time if unknown.
	// Runs due while it was off don't count as missed.
	Booted time.Time
}

// Local returns the environment of the machine Turnus is running on.
func Local() Env {
	home, _ := os.UserHomeDir()
	return Env{
		GOOS:           runtime.GOOS,
		Home:           home,
		UID:            os.Getuid(),
		Now:            time.Now(),
		Loc:            time.Local,
		Stat:           os.Stat,
		HasMail:        func() bool { return exists("/usr/sbin/sendmail") },
		DebianRunParts: exists("/etc/debian_version"),
		Running:        running(),
		CronPath:       cronPath(),
		LookPath:       exec.LookPath,
		Booted:         booted(),
	}
}

// booted returns when the machine started: from kern.boottime on macOS and
// the btime line of /proc/stat on Linux.
func booted() time.Time {
	var sec int64
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "kern.boottime").Output()
		if err != nil {
			return time.Time{}
		}
		// { sec = 1792171821, usec = 95062 } Fri Oct 16 13:30:21 2026
		if _, err := fmt.Sscanf(string(out), "{ sec = %d,", &sec); err != nil {
			return time.Time{}
		}
	case "linux":
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return time.Time{}
		}
		for line := range strings.Lines(string(data)) {
			if _, err := fmt.Sscanf(line, "btime %d", &sec); err == nil {
				break
			}
		}
	}
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// cronPath returns the PATH cron sets for jobs. cronie, the cron of Fedora
// and Red Hat, adds the sbin folders; Debian's cron and macOS's don't.
func cronPath() string {
	if runtime.GOOS == "linux" && exists("/usr/sbin/crond") {
		return "/usr/bin:/bin:/usr/sbin:/sbin"
	}
	return "/usr/bin:/bin"
}

// running returns a lookup that asks pgrep once per process name, since
// several jobs can depend on the same app.
func running() func(string) bool {
	var (
		mu   sync.Mutex
		seen = map[string]bool{}
	)
	return func(name string) bool {
		mu.Lock()
		defer mu.Unlock()
		if up, ok := seen[name]; ok {
			return up
		}
		up := exec.Command("pgrep", "-x", name).Run() == nil
		seen[name] = up
		return up
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type checker func(*job.Job, Env) *job.Problem

// Order matters: lastRun stays quiet about exit 78 when an earlier check has
// already explained it.
var checkers = []checker{
	invalidDefinition,
	noProgram,
	programMissing,
	notExecutable,
	notInPath,
	logDirMissing,
	lastRun,
	unloaded,
	neverRuns,
	missedWhileAsleep,
	daylightSaving,
	protectedFolder,
	output,
	percent,
	skippedByRunParts,
	notRunning,
	staleLog,
	agentSchedule,
	lastResult,
	hostRequired,
}

// Run records each job's problems, worst first. Jobs that ship with the OS
// are skipped.
func Run(jobs []job.Job, env Env) {
	for i := range jobs {
		j := &jobs[i]
		if j.Builtin {
			continue
		}
		for _, c := range checkers {
			if p := c(j, env); p != nil {
				j.Problems = append(j.Problems, *p)
			}
		}
		slices.SortStableFunc(j.Problems, func(a, b job.Problem) int {
			return cmp.Compare(b.Severity, a.Severity)
		})
	}
}

// tilde shortens paths under the home directory.
func (e Env) tilde(path string) string {
	if e.Home != "" && strings.HasPrefix(path, e.Home+"/") {
		return "~" + path[len(e.Home):]
	}
	return path
}

func hasFail(j *job.Job) bool {
	return slices.ContainsFunc(j.Problems, func(p job.Problem) bool {
		return p.Severity == job.Fail
	})
}
