package check

import (
	"fmt"
	"syscall"

	"github.com/sanifhimani/turnus/internal/job"
)

var signalNames = map[syscall.Signal]string{
	syscall.SIGHUP:  "SIGHUP",
	syscall.SIGINT:  "SIGINT",
	syscall.SIGQUIT: "SIGQUIT",
	syscall.SIGILL:  "SIGILL",
	syscall.SIGTRAP: "SIGTRAP",
	syscall.SIGABRT: "SIGABRT",
	syscall.SIGBUS:  "SIGBUS",
	syscall.SIGFPE:  "SIGFPE",
	syscall.SIGKILL: "SIGKILL",
	syscall.SIGSEGV: "SIGSEGV",
	syscall.SIGPIPE: "SIGPIPE",
	syscall.SIGTERM: "SIGTERM",
}

// lastRun reports a failed or crashed last run. launchd records a signal as
// a negative exit status.
func lastRun(j *job.Job, env Env) *job.Problem {
	if j.LastExit == nil || *j.LastExit == 0 {
		return nil
	}
	code := *j.LastExit
	fix := "Run it by hand to see the error"
	switch {
	case j.LogPath != "":
		fix = "Check the log at " + env.tilde(j.LogPath)
	case j.Unit != "":
		fix = "See its output with journalctl -e -u " + j.Unit
		if j.Scope == job.User {
			fix = "See its output with journalctl --user -e -u " + j.Unit
		}
	}

	if code < 0 {
		sig := syscall.Signal(-code)
		name := signalNames[sig]
		if name == "" {
			name = fmt.Sprintf("signal %d", -code)
		}
		if sig == syscall.SIGKILL || sig == syscall.SIGTERM {
			return &job.Problem{
				Code:     "killed",
				Severity: job.Warn,
				Summary:  fmt.Sprintf("Was stopped by %s on its last run", name),
				Fix:      fix,
			}
		}
		return &job.Problem{
			Code:     "crashed",
			Severity: job.Fail,
			Summary:  fmt.Sprintf("Crashed on its last run: %s (%s)", sig, name),
			Fix:      fix,
		}
	}

	summary := fmt.Sprintf("Last run failed with exit code %d", code)
	switch {
	case code == 78 && j.Source == job.Launchd:
		if hasFail(j) {
			return nil
		}
		summary = "launchd couldn't start it (exit 78, bad configuration)"
		fix = "Check the paths and keys in " + env.tilde(j.File)
	case code == 203 && j.Source == job.Systemd:
		if hasFail(j) {
			return nil
		}
		summary = "systemd couldn't run the program (exit 203)"
		fix = "Check ExecStart in " + j.Unit
	case code == 126:
		summary = "Last run failed: command isn't executable (exit 126)"
	case code == 127:
		summary = "Last run failed: command not found (exit 127)"
	}
	return &job.Problem{Code: "failed", Severity: job.Fail, Summary: summary, Fix: fix}
}

func unloaded(j *job.Job, env Env) *job.Problem {
	if j.State != job.StateUnloaded || j.Err != "" || j.Command == "" {
		return nil
	}
	switch j.Source {
	case job.Launchd:
		fix := fmt.Sprintf("launchctl bootstrap gui/%d %s", env.UID, shellPath(env.tilde(j.File)))
		if j.Scope == job.System {
			fix = "sudo launchctl bootstrap system " + shellPath(j.File)
		}
		return &job.Problem{
			Code:     "unloaded",
			Severity: job.Warn,
			Summary:  "Not loaded, so it never runs",
			Fix:      fix,
		}
	case job.Systemd:
		fix := "sudo systemctl start " + j.Name + ".timer"
		if j.Scope == job.User {
			fix = "systemctl --user start " + j.Name + ".timer"
		}
		return &job.Problem{
			Code:     "unloaded",
			Severity: job.Warn,
			Summary:  "Timer is enabled but not started, so it never runs",
			Fix:      fix,
		}
	}
	return nil
}
