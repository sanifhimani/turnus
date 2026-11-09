package check

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/sanifhimani/turnus/internal/job"
)

func invalidDefinition(j *job.Job, env Env) *job.Problem {
	if j.Err == "" {
		return nil
	}
	if j.Source == job.Systemd {
		return &job.Problem{
			Code:     "missing-unit",
			Severity: job.Fail,
			Summary:  "Timer starts " + j.Unit + ", which doesn't exist",
			Fix:      "Create " + j.Unit + ", or remove the timer",
		}
	}
	fix := "Run plutil -lint " + env.tilde(j.File) + " to see the error"
	if loaded(j) {
		return &job.Problem{
			Code:     "invalid-plist",
			Severity: job.Warn,
			Summary:  "Plist is invalid, though launchd loaded it anyway",
			Fix:      fix,
		}
	}
	return &job.Problem{
		Code:     "invalid-plist",
		Severity: job.Fail,
		Summary:  "Plist is invalid, so launchd can't load it",
		Fix:      fix,
	}
}

func noProgram(j *job.Job, env Env) *job.Problem {
	if j.Source != job.Launchd || j.Err != "" || j.Command != "" {
		return nil
	}
	return &job.Problem{
		Code:     "no-program",
		Severity: job.Warn,
		Summary:  "Defines no program, so there is nothing to run",
		Fix:      "Delete " + env.tilde(j.File) + " if the app that installed it is gone",
	}
}

func programMissing(j *job.Job, env Env) *job.Problem {
	if !filepath.IsAbs(j.Program) {
		return nil
	}
	if _, err := env.Stat(j.Program); !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return &job.Problem{
		Code:     "program-missing",
		Severity: job.Fail,
		Summary:  "Program not found: " + env.tilde(j.Program),
		Fix:      "Fix the path, or remove the job if it's no longer needed",
	}
}

func notExecutable(j *job.Job, env Env) *job.Problem {
	if j.Interpreted || !filepath.IsAbs(j.Program) {
		return nil
	}
	fi, err := env.Stat(j.Program)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o111 != 0 {
		return nil
	}
	return &job.Problem{
		Code:     "not-executable",
		Severity: job.Fail,
		Summary:  "Program isn't executable: " + env.tilde(j.Program),
		Fix:      "chmod +x " + shellPath(env.tilde(j.Program)),
	}
}

func logDirMissing(j *job.Job, env Env) *job.Problem {
	if j.LogPath == "" {
		return nil
	}
	dir := filepath.Dir(j.LogPath)
	if _, err := env.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	summary := "Log folder is missing, so the command never starts"
	if j.Source == job.Launchd {
		summary = "Log folder is missing, so launchd won't start it"
	}
	return &job.Problem{
		Code:     "log-dir-missing",
		Severity: job.Fail,
		Summary:  summary,
		Fix:      "mkdir -p " + shellPath(env.tilde(dir)),
	}
}

func loaded(j *job.Job) bool {
	return j.State == job.StateRunning || j.State == job.StateIdle
}

// shellPath quotes a path for pasting into a shell, leaving a leading ~ free
// to expand.
func shellPath(p string) string {
	if !strings.ContainsAny(p, " '\"$&()`;|<>*?") {
		return p
	}
	prefix := ""
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		prefix, p = "~/", rest
	}
	return prefix + "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
}
