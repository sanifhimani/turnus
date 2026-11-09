// Package job defines the normalized view of a scheduled job, regardless of
// which scheduler (cron, launchd, systemd) defines it.
package job

import "time"

// Source is the scheduler that owns a job.
type Source string

const (
	Cron    Source = "cron"
	Launchd Source = "launchd"
	Systemd Source = "systemd"
)

// Scope says whether a job runs as the current user or system-wide.
type Scope string

const (
	User   Scope = "user"
	System Scope = "system"
)

// Kind is whether a job runs on a schedule or as a long-running service.
type Kind string

const (
	Scheduled Kind = "scheduled"
	Service   Kind = "service"
)

// State is what the scheduler currently knows about a job.
type State string

const (
	StateUnknown  State = "unknown"
	StateRunning  State = "running"
	StateIdle     State = "idle"
	StateUnloaded State = "unloaded"
	StateDisabled State = "disabled"
)

// Severity is how serious a problem is.
type Severity int

const (
	Info Severity = iota
	Warn
	Fail
)

var severityNames = [...]string{"info", "warn", "fail"}

func (s Severity) String() string {
	if s < 0 || int(s) >= len(severityNames) {
		return "none"
	}
	return severityNames[s]
}

func (s Severity) MarshalText() ([]byte, error) { return []byte(s.String()), nil }

// Problem is an issue found with a job.
type Problem struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	Summary  string   `json:"summary"`
	Fix      string   `json:"fix,omitempty"`
}

// Job is one scheduled or managed command on this machine.
type Job struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Source  Source `json:"source"`
	Scope   Scope  `json:"scope"`
	Kind    Kind   `json:"kind"`
	Builtin bool   `json:"builtin"` // ships with the OS

	Command string            `json:"command"`
	Program string            `json:"program,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// Interpreted is set when Program is a script handed to an interpreter,
	// as in "sh backup.sh", so it does not need to be executable.
	Interpreted bool `json:"-"`

	Schedule string    `json:"schedule"`
	Expr     string    `json:"expr,omitempty"`
	Next     time.Time `json:"next,omitzero"`

	State    State `json:"state"`
	PID      int   `json:"pid,omitempty"`
	LastExit *int  `json:"last_exit,omitempty"`

	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	LogPath string `json:"log,omitempty"`

	// Unit is the systemd service a timer starts. Its output is in the
	// journal under this name.
	Unit string `json:"unit,omitempty"`

	// Persistent is set for systemd timers that run missed jobs after the
	// machine was off.
	Persistent bool `json:"-"`

	// Err is set when the job's definition is invalid or could not be read.
	Err string `json:"error,omitempty"`

	// Unreadable is set when turnus lacks permission to read the job's file,
	// so the jobs in it can't be shown.
	Unreadable bool `json:"unreadable,omitempty"`

	Problems []Problem `json:"problems,omitempty"`
}

// Worst returns the highest severity among the job's problems, or -1 when
// the job has none.
func (j *Job) Worst() Severity {
	worst := Severity(-1)
	for _, p := range j.Problems {
		worst = max(worst, p.Severity)
	}
	return worst
}
