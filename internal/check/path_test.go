package check

import (
	"errors"
	"testing"

	"github.com/sanifhimani/turnus/internal/job"
)

// The user's shell finds node and docker under /opt/homebrew/bin, outside
// cron's PATH.
func pathEnv() Env {
	env := testEnv(fakeFS{
		"/usr/bin/rsync":           0o755,
		"/usr/sbin/crontool":       0o755,
		"/opt/homebrew/bin/node":   0o755,
		"/opt/homebrew/bin/docker": 0o755,
		"/usr/local/bin/":          0,
	})
	env.CronPath = "/usr/bin:/bin"
	env.LookPath = func(name string) (string, error) {
		switch name {
		case "node", "docker":
			return "/opt/homebrew/bin/" + name, nil
		case "rsync":
			return "/usr/bin/rsync", nil
		}
		return "", errors.New("not found")
	}
	return env
}

func TestNotInPath(t *testing.T) {
	tests := []struct {
		name    string
		job     job.Job
		summary string
		fix     string
	}{
		{
			name:    "in cron's PATH",
			job:     job.Job{Source: job.Cron, Lookups: []string{"rsync"}},
			summary: "",
		},
		{
			name:    "only in the user's PATH",
			job:     job.Job{Source: job.Cron, Lookups: []string{"node"}},
			summary: `node isn't in cron's PATH, so the job fails with "command not found"`,
			fix:     "Use the full path /opt/homebrew/bin/node, or add PATH=/opt/homebrew/bin:/usr/bin:/bin above the job",
		},
		{
			name:    "several missing",
			job:     job.Job{Source: job.Cron, Lookups: []string{"rsync", "node", "docker"}},
			summary: `node and docker aren't in cron's PATH, so the job fails with "command not found"`,
			fix:     "Add PATH=/opt/homebrew/bin:/usr/bin:/bin above the job",
		},
		{
			name: "crontab sets a PATH that misses it",
			job: job.Job{Source: job.Cron, Lookups: []string{"node"},
				SearchPath: "/usr/local/bin:/usr/bin:/bin"},
			summary: `node isn't in cron's PATH, so the job fails with "command not found"`,
			fix:     "Use the full path /opt/homebrew/bin/node, or change the PATH line above the job to PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin",
		},
		{
			name:    "crontab sets a PATH that has it",
			job:     job.Job{Source: job.Cron, Lookups: []string{"node"}, SearchPath: "/opt/homebrew/bin:/usr/bin:/bin"},
			summary: "",
		},
		{
			// cron doesn't expand variables, so $PATH is a folder named "$PATH".
			name:    "PATH line relies on $PATH",
			job:     job.Job{Source: job.Cron, Lookups: []string{"rsync"}, SearchPath: "/opt/homebrew/bin:$PATH"},
			summary: `rsync isn't in cron's PATH, so the job fails with "command not found"`,
			fix:     "Use the full path /usr/bin/rsync, or change the PATH line above the job to PATH=/usr/bin:/opt/homebrew/bin:$PATH",
		},
		{
			name:    "not installed anywhere",
			job:     job.Job{Source: job.Cron, Lookups: []string{"restic"}},
			summary: `restic isn't in cron's PATH, so the job fails with "command not found"`,
			fix:     "Install restic, or use the full path",
		},
		{
			name:    "launchd searches the sbin folders",
			job:     job.Job{Source: job.Launchd, Lookups: []string{"crontool"}},
			summary: "",
		},
		{
			name:    "launchd bare program",
			job:     job.Job{Source: job.Launchd, Lookups: []string{"node"}},
			summary: "node isn't in launchd's PATH, so launchd can't start it",
			fix:     "Use the full path /opt/homebrew/bin/node in ProgramArguments",
		},
		{
			name:    "systemd isn't checked",
			job:     job.Job{Source: job.Systemd, Lookups: []string{"node"}},
			summary: "",
		},
		{
			name:    "invalid job",
			job:     job.Job{Source: job.Cron, Lookups: []string{"node"}, Err: "its schedule is invalid"},
			summary: "",
		},
	}
	for _, tt := range tests {
		p := notInPath(&tt.job, pathEnv())
		switch {
		case tt.summary == "" && p != nil:
			t.Errorf("%s: got %q, want no problem", tt.name, p.Summary)
		case tt.summary != "" && p == nil:
			t.Errorf("%s: got no problem, want %q", tt.name, tt.summary)
		case p != nil && (p.Summary != tt.summary || p.Fix != tt.fix || p.Severity != job.Fail):
			t.Errorf("%s:\n got  %q / %q\n want %q / %q", tt.name, p.Summary, p.Fix, tt.summary, tt.fix)
		}
	}
}

// A launchd job whose bare program can't be found exits 78. The PATH problem
// explains it, so the generic exit 78 message stays quiet.
func TestNotInPathExplainsExit78(t *testing.T) {
	jobs := []job.Job{{Source: job.Launchd, Kind: job.Scheduled, Command: "node /Users/me/sync.js",
		Program: "node", Lookups: []string{"node"}, State: job.StateIdle, LastExit: exit(78)}}
	Run(jobs, pathEnv())
	if got := codes(jobs[0]); len(got) != 1 || got[0] != "not-in-path" {
		t.Errorf("codes = %q, want only not-in-path", got)
	}
}
