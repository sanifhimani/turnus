package check

import (
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

// file is a fake file with a modified time and size.
type file struct {
	mod  time.Time
	size int64
	dir  bool
}

type fileInfo struct {
	os.FileInfo
	f file
}

func (i fileInfo) ModTime() time.Time { return i.f.mod }
func (i fileInfo) Size() int64        { return i.f.size }
func (i fileInfo) Mode() fs.FileMode {
	if i.f.dir {
		return fs.ModeDir | 0o755
	}
	return 0o755
}

// staleEnv is a Linux machine at noon on Sunday Nov 8, booted a month ago.
// The crontab was saved on Oct 20.
func staleEnv(files map[string]file) Env {
	env := testEnv(nil)
	env.GOOS = "linux"
	env.Booted = env.Now.AddDate(0, -1, 0)
	if _, ok := files["/var/spool/cron/crontabs/me"]; !ok {
		files["/var/spool/cron/crontabs/"] = file{mod: time.Date(2026, 10, 20, 9, 0, 0, 0, env.Loc), dir: true}
	}
	env.Stat = func(path string) (os.FileInfo, error) {
		if f, ok := files[path]; ok {
			return fileInfo{f: f}, nil
		}
		if f, ok := files[path+"/"]; ok {
			return fileInfo{f: f}, nil
		}
		return nil, &fs.PathError{Op: "stat", Path: path, Err: fs.ErrNotExist}
	}
	return env
}

func cronJob(expr, log string) job.Job {
	return job.Job{Source: job.Cron, Kind: job.Scheduled, Expr: expr, State: job.StateIdle,
		Command: "/opt/backup.sh >> " + log + " 2>&1", Program: "/opt/backup.sh",
		File: "/var/spool/cron/crontabs/me", LogPath: log}
}

func TestStaleLog(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	at := func(month time.Month, d, h int) time.Time { return time.Date(2026, month, d, h, 0, 0, 0, ny) }

	tests := []struct {
		name  string
		job   job.Job
		files map[string]file
		want  string // start of the summary, or "" for no problem
	}{
		{
			name:  "written after the last run",
			job:   cronJob("0 3 * * *", "/var/log/b.log"),
			files: map[string]file{"/var/log/b.log": {mod: at(11, 8, 3), size: 10}},
		},
		{
			name:  "two missed daily runs is not enough",
			job:   cronJob("0 3 * * *", "/var/log/b.log"),
			files: map[string]file{"/var/log/b.log": {mod: at(11, 6, 3), size: 10}},
		},
		{
			name:  "three missed daily runs",
			job:   cronJob("0 3 * * *", "/var/log/b.log"),
			files: map[string]file{"/var/log/b.log": {mod: at(11, 5, 3), size: 10}},
			want:  "Nothing written to its log since Nov 5, though it was due to run 3 times",
		},
		{
			name:  "hourly job quiet for a day",
			job:   cronJob("0 * * * *", "/var/log/b.log"),
			files: map[string]file{"/var/log/b.log": {mod: at(11, 7, 12), size: 10}},
			want:  "Nothing written to its log since Nov 7, though it was due to run 24 times",
		},
		{
			name:  "minutely job quiet for under an hour",
			job:   cronJob("* * * * *", "/var/log/b.log"),
			files: map[string]file{"/var/log/b.log": {mod: at(11, 8, 12).Add(-20 * time.Minute), size: 10}},
		},
		{
			name:  "minutely job quiet for a month",
			job:   cronJob("* * * * *", "/var/log/b.log"),
			files: map[string]file{"/var/log/b.log": {mod: at(10, 1, 0), size: 10}},
			want:  "Nothing written to its log since Oct 1, though it was due to run more than 10000 times",
		},
		{
			name:  "empty log says nothing",
			job:   cronJob("0 3 * * *", "/var/log/b.log"),
			files: map[string]file{"/var/log/b.log": {mod: at(10, 1, 3)}},
		},
		{
			// The machine was off until a month ago, so older runs don't count.
			name: "runs before boot don't count",
			job:  cronJob("0 3 * * 1", "/var/log/b.log"),
			files: map[string]file{
				"/var/log/b.log":              {mod: time.Date(2025, 6, 1, 0, 0, 0, 0, ny), size: 10},
				"/var/spool/cron/crontabs/me": {mod: time.Date(2025, 1, 1, 0, 0, 0, 0, ny)},
			},
			want: "Nothing written to its log since Jun 1, 2025, though it was due to run 4 times",
		},
		{
			// A new job pointing at an old log: only runs since the crontab
			// was saved count.
			name: "crontab saved after the log",
			job:  cronJob("0 3 * * *", "/var/log/b.log"),
			files: map[string]file{
				"/var/log/b.log":              {mod: at(9, 1, 3), size: 10},
				"/var/spool/cron/crontabs/me": {mod: at(11, 7, 9)},
			},
		},
		{
			name: "log never created",
			job:  cronJob("0 3 * * *", "/var/log/b.log"),
			files: map[string]file{
				"/var/log/":                   {dir: true},
				"/var/spool/cron/crontabs/me": {mod: at(11, 1, 9)},
			},
			want: "Its log was never created, though it was due to run 7 times since Nov 1",
		},
		{
			name: "log never created, but the job is new",
			job:  cronJob("0 3 * * *", "/var/log/b.log"),
			files: map[string]file{
				"/var/log/":                   {dir: true},
				"/var/spool/cron/crontabs/me": {mod: at(11, 7, 9)},
			},
		},
		{
			name:  "launchd StartInterval",
			job:   job.Job{Source: job.Launchd, Kind: job.Scheduled, Expr: "StartInterval 3600", State: job.StateIdle, LogPath: "/Users/me/Library/Logs/sync.log", File: "/Users/me/Library/LaunchAgents/sync.plist"},
			files: map[string]file{"/Users/me/Library/Logs/sync.log": {mod: at(11, 8, 6), size: 10}},
			want:  "Nothing written to its log since Nov 8, though it was due to run 6 times",
		},
		{
			name:  "unloaded launchd job is left to the unloaded check",
			job:   job.Job{Source: job.Launchd, Kind: job.Scheduled, Expr: "StartInterval 3600", State: job.StateUnloaded, LogPath: "/Users/me/Library/Logs/sync.log"},
			files: map[string]file{"/Users/me/Library/Logs/sync.log": {mod: at(11, 1, 6), size: 10}},
		},
		{
			name: "launchd ran after the log changed",
			job: job.Job{Source: job.Launchd, Kind: job.Scheduled, Expr: "StartInterval 3600", State: job.StateIdle,
				LogPath: "/Users/me/Library/Logs/sync.log", LastRun: at(11, 8, 11)},
			files: map[string]file{"/Users/me/Library/Logs/sync.log": {mod: at(11, 1, 6), size: 10}},
		},
		{
			name:  "@reboot has no schedule to miss",
			job:   cronJob("@reboot", "/var/log/b.log"),
			files: map[string]file{"/var/log/b.log": {mod: at(10, 1, 3), size: 10}},
		},
		{
			name:  "log to a device",
			job:   cronJob("0 3 * * *", "/dev/console"),
			files: map[string]file{},
		},
	}
	for _, tt := range tests {
		p := staleLog(&tt.job, staleEnv(tt.files))
		switch {
		case tt.want == "" && p != nil:
			t.Errorf("%s: got %q, want no problem", tt.name, p.Summary)
		case tt.want != "" && p == nil:
			t.Errorf("%s: got no problem, want %q", tt.name, tt.want)
		case p != nil && (!strings.HasPrefix(p.Summary, tt.want) || p.Severity != job.Warn):
			t.Errorf("%s:\n got  %q\n want %q", tt.name, p.Summary, tt.want)
		}
	}
}

func TestStaleLogFix(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	files := map[string]file{"/var/log/b.log": {mod: time.Date(2026, 11, 1, 3, 0, 0, 0, ny), size: 10}}

	j := cronJob("0 3 * * *", "/var/log/b.log")
	p := staleLog(&j, staleEnv(files))
	if p == nil || p.Fix != "If it writes only on errors, ignore this. Otherwise run it by hand to see why it isn't running" {
		t.Fatalf("linux fix = %+v", p)
	}

	env := staleEnv(files)
	env.GOOS = "darwin"
	j = cronJob("0 3 * * *", "/var/log/b.log")
	if p = staleLog(&j, env); p == nil || !strings.HasSuffix(p.Fix, ". cron also skips runs while the Mac is asleep") {
		t.Errorf("darwin fix = %+v", p)
	}
}

// A failure already explains why nothing was written.
func TestStaleLogQuietAfterFailure(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	files := map[string]file{
		"/var/log/b.log": {mod: time.Date(2026, 11, 1, 3, 0, 0, 0, ny), size: 10},
	}
	j := cronJob("0 3 * * *", "/var/log/b.log")
	j.Problems = []job.Problem{{Code: "program-missing", Severity: job.Fail}}
	if p := staleLog(&j, staleEnv(files)); p != nil {
		t.Errorf("got %q, want no problem after a failure", p.Summary)
	}
}

func TestNotRunning(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	at := func(d, h int) time.Time { return time.Date(2026, 11, d, h, 0, 0, 0, ny) }
	since := at(1, 0)

	tests := []struct {
		name string
		job  job.Job
		want string
	}{
		{
			name: "ran recently",
			job:  job.Job{Source: job.Cron, Kind: job.Scheduled, Expr: "0 3 * * *", HistorySince: since, LastRun: at(8, 3)},
		},
		{
			name: "never ran while the log covers it",
			job:  job.Job{Source: job.Cron, Kind: job.Scheduled, Expr: "0 3 * * *", HistorySince: since},
			want: "cron's log shows no runs since Nov 1, though it was due to run 8 times",
		},
		{
			name: "stopped running",
			job:  job.Job{Source: job.Cron, Kind: job.Scheduled, Expr: "0 * * * *", HistorySince: since, LastRun: at(7, 12)},
			want: "Last ran Nov 7, though it was due to run 24 times since",
		},
		{
			name: "no log to go by",
			job:  job.Job{Source: job.Cron, Kind: job.Scheduled, Expr: "0 3 * * *"},
		},
		{
			name: "a failure already explains it",
			job: job.Job{Source: job.Cron, Kind: job.Scheduled, Expr: "0 3 * * *", HistorySince: since,
				Problems: []job.Problem{{Code: "not-in-path", Severity: job.Fail}}},
		},
	}
	for _, tt := range tests {
		p := notRunning(&tt.job, staleEnv(map[string]file{}))
		switch {
		case tt.want == "" && p != nil:
			t.Errorf("%s: got %q, want no problem", tt.name, p.Summary)
		case tt.want != "" && p == nil:
			t.Errorf("%s: got no problem, want %q", tt.name, tt.want)
		case p != nil && p.Summary != tt.want:
			t.Errorf("%s:\n got  %q\n want %q", tt.name, p.Summary, tt.want)
		}
	}
}

// A job cron's log shows isn't running gets one problem, not two.
func TestNotRunningReplacesStaleLog(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	files := map[string]file{
		"/var/log/b.log": {mod: time.Date(2026, 11, 1, 3, 0, 0, 0, ny), size: 10},
		"/var/log/":      {dir: true},
		"/opt/backup.sh": {size: 100},
	}
	j := cronJob("0 3 * * *", "/var/log/b.log")
	j.HistorySince = time.Date(2026, 11, 1, 0, 0, 0, 0, ny)
	jobs := []job.Job{j}
	Run(jobs, staleEnv(files))
	if got := codes(jobs[0]); !slices.Contains(got, "not-running") || slices.Contains(got, "stale-log") {
		t.Errorf("codes = %q", got)
	}
}

// When cron's log shows a recent run, a quiet log is just a quiet job.
func TestRecentRunQuietsStaleLog(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	files := map[string]file{
		"/var/log/b.log": {mod: time.Date(2026, 11, 1, 3, 0, 0, 0, ny), size: 10},
		"/var/log/":      {dir: true},
		"/opt/backup.sh": {size: 100},
	}
	j := cronJob("0 3 * * *", "/var/log/b.log")
	j.HistorySince = time.Date(2026, 11, 1, 0, 0, 0, 0, ny)
	j.LastRun = time.Date(2026, 11, 8, 3, 0, 0, 0, ny)
	jobs := []job.Job{j}
	Run(jobs, staleEnv(files))
	if got := codes(jobs[0]); slices.Contains(got, "stale-log") || slices.Contains(got, "not-running") {
		t.Errorf("codes = %q", got)
	}
}
