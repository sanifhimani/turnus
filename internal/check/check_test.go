package check

import (
	"io/fs"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

// fakeFS maps paths to permissions. Directories end in a slash.
type fakeFS map[string]fs.FileMode

func (f fakeFS) stat(path string) (os.FileInfo, error) {
	if mode, ok := f[path]; ok {
		return fakeInfo{mode: mode}, nil
	}
	if _, ok := f[path+"/"]; ok {
		return fakeInfo{mode: fs.ModeDir | 0o755}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: path, Err: fs.ErrNotExist}
}

type fakeInfo struct {
	os.FileInfo
	mode fs.FileMode
}

func (i fakeInfo) Mode() fs.FileMode { return i.mode }

func testEnv(files fakeFS) Env {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	return Env{
		GOOS: "darwin",
		Home: "/Users/me",
		UID:  501,
		Now:  time.Date(2026, 11, 8, 12, 0, 0, 0, ny),
		Loc:  ny,
		Stat: files.stat,
	}
}

func exit(n int) *int { return &n }

func codes(j job.Job) []string {
	var out []string
	for _, p := range j.Problems {
		out = append(out, p.Code)
	}
	return out
}

func TestRun(t *testing.T) {
	files := fakeFS{
		"/usr/bin/true":    0o755,
		"/Users/me/script": 0o644,
		"/Users/me/logs/":  0,
		"/opt/run.sh":      0o644,
	}

	tests := []struct {
		name string
		job  job.Job
		want []string
	}{
		{
			name: "healthy",
			job: job.Job{Source: job.Launchd, Kind: job.Scheduled, Command: "/usr/bin/true", Program: "/usr/bin/true",
				Expr: "0 9 * * *", State: job.StateIdle, LastExit: exit(0)},
			want: nil,
		},
		{
			name: "program missing explains exit 78",
			job: job.Job{Source: job.Launchd, Command: "/nope", Program: "/nope",
				State: job.StateIdle, LastExit: exit(78)},
			want: []string{"program-missing"},
		},
		{
			name: "unexplained exit 78",
			job: job.Job{Source: job.Launchd, Command: "/usr/bin/true", Program: "/usr/bin/true",
				State: job.StateIdle, LastExit: exit(78)},
			want: []string{"failed"},
		},
		{
			name: "not executable",
			job:  job.Job{Source: job.Launchd, Command: "/Users/me/script", Program: "/Users/me/script", State: job.StateIdle},
			want: []string{"not-executable"},
		},
		{
			name: "interpreted script needs no exec bit",
			job: job.Job{Source: job.Cron, Kind: job.Scheduled, Command: "sh /opt/run.sh >> /Users/me/logs/run.log",
				Program: "/opt/run.sh", Interpreted: true, Expr: "*/5 * * * *", LogPath: "/Users/me/logs/run.log"},
			want: nil,
		},
		{
			name: "log folder missing",
			job: job.Job{Source: job.Launchd, Command: "/usr/bin/true", Program: "/usr/bin/true",
				LogPath: "/Users/me/gone/out.log", State: job.StateIdle},
			want: []string{"log-dir-missing"},
		},
		{
			name: "crashed",
			job: job.Job{Source: job.Launchd, Command: "/usr/bin/true", Program: "/usr/bin/true",
				State: job.StateIdle, LastExit: exit(-11)},
			want: []string{"crashed"},
		},
		{
			name: "killed",
			job: job.Job{Source: job.Launchd, Command: "/usr/bin/true", Program: "/usr/bin/true",
				State: job.StateIdle, LastExit: exit(-9)},
			want: []string{"killed"},
		},
		{
			name: "unloaded",
			job: job.Job{Source: job.Launchd, Command: "/usr/bin/true", Program: "/usr/bin/true",
				State: job.StateUnloaded, File: "/Users/me/Library/LaunchAgents/x.plist"},
			want: []string{"unloaded"},
		},
		{
			name: "empty plist",
			job:  job.Job{Source: job.Launchd, State: job.StateUnloaded},
			want: []string{"no-program"},
		},
		{
			name: "invalid plist",
			job:  job.Job{Source: job.Launchd, Err: "bad xml", State: job.StateUnloaded},
			want: []string{"invalid-plist"},
		},
		{
			name: "never runs",
			job: job.Job{Source: job.Cron, Kind: job.Scheduled, Command: "/usr/bin/true >> /Users/me/logs/x.log",
				Program: "/usr/bin/true", Expr: "0 0 30 2 *", LogPath: "/Users/me/logs/x.log"},
			want: []string{"never-runs"},
		},
		{
			name: "cron at 2:30 on a laptop",
			job: job.Job{Source: job.Cron, Kind: job.Scheduled, Command: "/usr/bin/true >> /Users/me/logs/x.log",
				Program: "/usr/bin/true", Expr: "30 2 * * *", LogPath: "/Users/me/logs/x.log"},
			want: []string{"missed-while-asleep", "daylight-saving"},
		},
		{
			name: "hourly StartInterval",
			job: job.Job{Source: job.Launchd, Kind: job.Scheduled, Command: "/usr/bin/true", Program: "/usr/bin/true",
				Expr: "StartInterval 3600", State: job.StateIdle},
			want: []string{"missed-while-asleep"},
		},
		{
			name: "cron reading Documents",
			job: job.Job{Source: job.Cron, Kind: job.Scheduled, Command: "/usr/bin/true ~/Documents/notes >/dev/null",
				Program: "/usr/bin/true", Expr: "*/5 * * * *"},
			want: []string{"protected-folder", "output"},
		},
		{
			name: "timer starts a missing service",
			job:  job.Job{Source: job.Systemd, Kind: job.Scheduled, Unit: "x.service", Err: "missing", State: job.StateUnloaded},
			want: []string{"missing-unit"},
		},
		{
			name: "program missing explains exit 203",
			job: job.Job{Source: job.Systemd, Kind: job.Scheduled, Command: "/nope", Program: "/nope",
				Unit: "x.service", State: job.StateIdle, LastExit: exit(203), Persistent: true},
			want: []string{"program-missing"},
		},
		{
			name: "timer enabled but not started",
			job: job.Job{Source: job.Systemd, Kind: job.Scheduled, Name: "x", Command: "/usr/bin/true",
				Program: "/usr/bin/true", State: job.StateUnloaded},
			want: []string{"unloaded"},
		},
		{
			name: "daily timer without Persistent",
			job: job.Job{Source: job.Systemd, Kind: job.Scheduled, Command: "/usr/bin/true", Program: "/usr/bin/true",
				Expr: "*-*-* 04:00:00", State: job.StateIdle},
			want: []string{"missed-while-off"},
		},
		{
			name: "persistent timer at 2:30",
			job: job.Job{Source: job.Systemd, Kind: job.Scheduled, Command: "/usr/bin/true", Program: "/usr/bin/true",
				Expr: "*-*-* 02:30:00", State: job.StateIdle, Persistent: true},
			want: []string{"daylight-saving"},
		},
		{
			name: "timer that never runs",
			job: job.Job{Source: job.Systemd, Kind: job.Scheduled, Command: "/usr/bin/true", Program: "/usr/bin/true",
				Expr: "*-02-30 00:00:00", State: job.StateIdle, Persistent: true},
			want: []string{"never-runs"},
		},
		{
			name: "scripts in cron.daily follow run-parts",
			job: job.Job{Source: job.Cron, Kind: job.Scheduled, Command: "/etc/cron.daily/x", Program: "/usr/bin/true",
				File: "/etc/cron.daily/x", Expr: "@daily"},
			want: []string{"missed-while-asleep"},
		},
		{
			name: "invalid cron line",
			job: job.Job{Source: job.Cron, Kind: job.Scheduled, Command: "* * * *", Line: 3,
				File: "/etc/cron.d/app", Err: "it needs a schedule and a command"},
			want: []string{"invalid-line"},
		},
		{
			name: "unreadable cron file",
			job:  job.Job{Source: job.Cron, Kind: job.Scheduled, File: "/etc/cron.d/secret", Err: "permission denied", Unreadable: true},
			want: []string{"unreadable"},
		},
		{
			name: "percent in a cron command",
			job: job.Job{Source: job.Cron, Kind: job.Scheduled, Command: "/usr/bin/true $(date +%F) >> /Users/me/logs/x.log",
				Program: "/usr/bin/true", Expr: "*/5 * * * *", LogPath: "/Users/me/logs/x.log"},
			want: []string{"unescaped-percent"},
		},
		{
			name: "builtin jobs are skipped",
			job:  job.Job{Source: job.Launchd, Builtin: true, State: job.StateUnloaded},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobs := []job.Job{tt.job}
			Run(jobs, testEnv(files))
			if got := codes(jobs[0]); !slices.Equal(got, tt.want) {
				t.Errorf("problems = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunSortsWorstFirst(t *testing.T) {
	jobs := []job.Job{{
		Source: job.Cron, Kind: job.Scheduled, Command: "/nope", Program: "/nope", Expr: "30 2 * * *",
	}}
	Run(jobs, testEnv(fakeFS{}))
	if got := jobs[0].Worst(); got != job.Fail {
		t.Fatalf("worst = %v, want fail", got)
	}
	if first := jobs[0].Problems[0].Code; first != "program-missing" {
		t.Errorf("first problem = %q, want program-missing", first)
	}
}

func TestOutput(t *testing.T) {
	tests := []struct {
		cmd  string
		env  map[string]string
		want string
	}{
		{"/bin/x", nil, "Output goes to local mail, which few people read"},
		{"/bin/x", map[string]string{"MAILTO": ""}, "Output is discarded because MAILTO is empty"},
		{"/bin/x", map[string]string{"MAILTO": "me@example.com"}, ""},
		{"/bin/x > /dev/null 2>&1", nil, "Output is thrown away, so errors leave no trace"},
		{"/bin/x 2>/dev/null", nil, "Output goes to local mail, which few people read"},
	}
	noMail := testEnv(nil)
	noMail.HasMail = func() bool { return false }
	if p := output(&job.Job{Source: job.Cron, Command: "/bin/x"}, noMail); p == nil || p.Summary != "Output is discarded because no mail server is installed" {
		t.Errorf("without mail: %+v", p)
	}
	for _, tt := range tests {
		j := &job.Job{Source: job.Cron, Name: "x", Command: tt.cmd, Env: tt.env}
		var got string
		if p := output(j, testEnv(nil)); p != nil {
			got = p.Summary
		}
		if got != tt.want {
			t.Errorf("output(%q, %v) = %q, want %q", tt.cmd, tt.env, got, tt.want)
		}
	}
}

func TestShellPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/usr/local/bin/x", "/usr/local/bin/x"},
		{"~/Library/Application Support/x", "~/'Library/Application Support/x'"},
		{"/tmp/it's", `'/tmp/it'\''s'`},
	}
	for _, tt := range tests {
		if got := shellPath(tt.in); got != tt.want {
			t.Errorf("shellPath(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestObservesDST(t *testing.T) {
	now := time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC)
	tokyo, _ := time.LoadLocation("Asia/Tokyo")
	berlin, _ := time.LoadLocation("Europe/Berlin")
	if observesDST(tokyo, now) {
		t.Error("Tokyo has no daylight saving")
	}
	if !observesDST(berlin, now) {
		t.Error("Berlin has daylight saving")
	}
	if observesDST(time.UTC, now) {
		t.Error("UTC has no daylight saving")
	}
}
