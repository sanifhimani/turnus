package launchd

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

func TestReadJob(t *testing.T) {
	now := time.Date(2026, 11, 8, 21, 30, 0, 0, time.Local) // a Sunday
	agent := location{dir: "testdata"}

	tests := []struct {
		file     string
		name     string
		kind     job.Kind
		schedule string
		command  string
		next     time.Time
	}{
		{
			file:     "calendar.plist",
			name:     "com.example.backup",
			kind:     job.Scheduled,
			schedule: "daily at 03:30",
			command:  "/usr/local/bin/backup --to '/Volumes/Backup Drive'",
			next:     time.Date(2026, 11, 9, 3, 30, 0, 0, time.Local),
		},
		{
			file:     "binary.plist",
			name:     "com.example.backup",
			kind:     job.Scheduled,
			schedule: "daily at 03:30",
			command:  "/usr/local/bin/backup --to '/Volumes/Backup Drive'",
			next:     time.Date(2026, 11, 9, 3, 30, 0, 0, time.Local),
		},
		{
			file:     "weekdays.plist",
			name:     "com.example.standup",
			kind:     job.Scheduled,
			schedule: "Mon and Wed at 09:00",
			command:  "/usr/local/bin/standup",
			next:     time.Date(2026, 11, 9, 9, 0, 0, 0, time.Local),
		},
		{
			file:     "interval.plist",
			name:     "com.example.updater",
			kind:     job.Scheduled,
			schedule: "hourly",
			command:  "/Applications/Example.app/Contents/MacOS/updater --wake",
		},
		{
			file:     "service.plist",
			name:     "com.example.db",
			kind:     job.Service,
			schedule: "always running",
			command:  "/opt/homebrew/opt/db/bin/db",
		},
		{
			file:     "empty.plist",
			name:     "empty",
			kind:     job.Service,
			schedule: "on demand",
		},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			j := readJob("testdata/"+tt.file, agent, now)
			if j.Name != tt.name || j.Kind != tt.kind || j.Schedule != tt.schedule || j.Command != tt.command {
				t.Errorf("got name=%q kind=%q schedule=%q command=%q", j.Name, j.Kind, j.Schedule, j.Command)
			}
			if !j.Next.Equal(tt.next) {
				t.Errorf("next = %v, want %v", j.Next, tt.next)
			}
		})
	}
}

func TestTriggerRunAtLoad(t *testing.T) {
	s := spec{RunAtLoad: true}
	if got := s.trigger(true, time.Now()).desc; got != "at boot" {
		t.Errorf("daemon trigger = %q, want at boot", got)
	}
	if got := s.trigger(false, time.Now()).desc; got != "at login" {
		t.Errorf("agent trigger = %q, want at login", got)
	}
}

func TestApplyStatus(t *testing.T) {
	exit1 := 1
	d := domain{
		services: map[string]service{
			"running": {pid: 42},
			"failed":  {lastExit: &exit1},
		},
		disabled: map[string]bool{"off": true, "on": false},
	}
	tests := []struct {
		name  string
		start job.State
		want  job.State
	}{
		{"running", job.StateUnknown, job.StateRunning},
		{"failed", job.StateUnknown, job.StateIdle},
		{"missing", job.StateUnknown, job.StateUnloaded},
		{"off", job.StateUnknown, job.StateDisabled},
		{"on", job.StateDisabled, job.StateUnloaded},
	}
	for _, tt := range tests {
		j := job.Job{Name: tt.name, State: tt.start}
		applyStatus(&j, d)
		if j.State != tt.want {
			t.Errorf("%s: state = %q, want %q", tt.name, j.State, tt.want)
		}
	}
}

func TestMergeCron(t *testing.T) {
	tests := []struct {
		in   []string
		want string
		ok   bool
	}{
		{[]string{"0 9 * * 1", "0 9 * * 3"}, "0 9 * * 1,3", true},
		{[]string{"0 9 * * *", "0 17 * * *"}, "0 9,17 * * *", true},
		{[]string{"0 9 * * 1", "30 17 * * 3"}, "", false},
		{[]string{"0 9 * * *"}, "", false},
	}
	for _, tt := range tests {
		got, ok := mergeCron(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("mergeCron(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestReadJobLookups(t *testing.T) {
	now := time.Date(2026, 11, 8, 21, 30, 0, 0, time.Local)
	agent := location{dir: "testdata"}

	// launchd searches its own fixed PATH for a bare first argument, so the
	// plist's EnvironmentVariables PATH doesn't count.
	j := readJob(filepath.Join("testdata", "bare.plist"), agent, now)
	if !slices.Equal(j.Lookups, []string{"node"}) || j.SearchPath != "" {
		t.Errorf("bare.plist: lookups = %q, path = %q", j.Lookups, j.SearchPath)
	}
	if j = readJob(filepath.Join("testdata", "interval.plist"), agent, now); j.Lookups != nil {
		t.Errorf("interval.plist: lookups = %q, want none for an absolute path", j.Lookups)
	}
}
