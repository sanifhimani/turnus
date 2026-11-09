// Package check finds problems with jobs: things that are broken, and things
// that will quietly not run when you expect them to.
package check

import (
	"cmp"
	"os"
	"runtime"
	"slices"
	"strings"
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
}

// Local returns the environment of the machine Turnus is running on.
func Local() Env {
	home, _ := os.UserHomeDir()
	return Env{
		GOOS: runtime.GOOS,
		Home: home,
		UID:  os.Getuid(),
		Now:  time.Now(),
		Loc:  time.Local,
		Stat: os.Stat,
	}
}

type checker func(*job.Job, Env) *job.Problem

// Order matters: lastRun stays quiet about exit 78 when an earlier check has
// already explained it.
var checkers = []checker{
	invalidDefinition,
	noProgram,
	programMissing,
	notExecutable,
	logDirMissing,
	lastRun,
	unloaded,
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
