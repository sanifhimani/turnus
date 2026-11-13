package check

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/sanifhimani/turnus/internal/job"
)

// launchdPath is _PATH_STDPATH, where launchd looks for a ProgramArguments
// program given by bare name.
const launchdPath = "/usr/bin:/bin:/usr/sbin:/sbin"

// notInPath reports programs a job runs by bare name that aren't in the PATH
// its scheduler gives it. A command that works in a terminal fails this way
// under cron, because cron's PATH leaves out folders such as /usr/local/bin
// and /opt/homebrew/bin.
func notInPath(j *job.Job, env Env) *job.Problem {
	if j.Err != "" || len(j.Lookups) == 0 {
		return nil
	}
	var who, path, consequence string
	switch j.Source {
	case job.Cron:
		who, path = "cron", cmp.Or(j.SearchPath, env.CronPath)
		consequence = `so the job fails with "command not found"`
	case job.Launchd:
		who, path = "launchd", launchdPath
		consequence = "so launchd can't start it"
	default:
		return nil
	}
	dirs := filepath.SplitList(path)

	var missing, found []string
	for _, name := range j.Lookups {
		if inDirs(name, dirs, env) {
			continue
		}
		missing = append(missing, name)
		if env.LookPath == nil {
			continue
		}
		if p, err := env.LookPath(name); err == nil && filepath.IsAbs(p) {
			found = append(found, p)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	verb := "isn't"
	if len(missing) > 1 {
		verb = "aren't"
	}
	return &job.Problem{
		Code:     "not-in-path",
		Severity: job.Fail,
		Summary:  fmt.Sprintf("%s %s in %s's PATH, %s", list(missing), verb, who, consequence),
		Fix:      pathFix(j, missing, found, dirs),
	}
}

// pathFix suggests the full path when turnus could find every missing
// program, and otherwise says what to look for.
func pathFix(j *job.Job, missing, found, dirs []string) string {
	if len(found) < len(missing) {
		if j.Source == job.Launchd {
			return "Use the full path to " + missing[0] + " in ProgramArguments"
		}
		return "Install " + list(missing) + ", or use the full path"
	}
	if j.Source == job.Launchd {
		return "Use the full path " + found[0] + " in ProgramArguments"
	}

	line := "PATH=" + strings.Join(widen(dirs, found), ":")
	verb := "add " + line + " above the job"
	if j.SearchPath != "" {
		verb = "change the PATH line above the job to " + line
	}
	if len(found) == 1 {
		return "Use the full path " + found[0] + ", or " + verb
	}
	return strings.ToUpper(verb[:1]) + verb[1:]
}

// widen puts the folders of the programs found ahead of the existing PATH.
func widen(dirs, programs []string) []string {
	var out []string
	for _, p := range programs {
		if d := filepath.Dir(p); !slices.Contains(out, d) && !slices.Contains(dirs, d) {
			out = append(out, d)
		}
	}
	return append(out, dirs...)
}

// inDirs reports whether one of the folders holds a file with this name.
// Only absolute folders count: cron doesn't expand $PATH or ~ in a PATH line.
func inDirs(name string, dirs []string, env Env) bool {
	for _, d := range dirs {
		if !filepath.IsAbs(d) {
			continue
		}
		if fi, err := env.Stat(filepath.Join(d, name)); err == nil && !fi.Mode().IsDir() {
			return true
		}
	}
	return false
}

// list joins names for a sentence: "a", "a and b", "a, b and c".
func list(names []string) string {
	if len(names) == 1 {
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}
