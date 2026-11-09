package crontab

import (
	"errors"
	"io/fs"
	"testing"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

var now = time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

func TestToJobInvalid(t *testing.T) {
	j := toJob(Entry{Line: 4, Command: "* * * *", Err: "it needs a schedule and a command"}, "/etc/cron.d/app", job.System, now)
	if j.Name != "line 4" || j.Err == "" || j.Schedule != "invalid" {
		t.Errorf("invalid line: %+v", j)
	}

	j = toJob(Entry{Line: 2, Expr: "60 * * * *", Command: "/bin/true"}, "/etc/cron.d/app", job.System, now)
	if j.Err != "its schedule is invalid (minute: 60 is out of range 0-59)" {
		t.Errorf("bad schedule: err = %q", j.Err)
	}
}

func TestToJobPercent(t *testing.T) {
	// cron cuts the command at %, so the shell really writes to /var/log/b-.
	j := toJob(Entry{Line: 1, Expr: "@daily", Command: "/usr/bin/backup >> /var/log/b-%F.log 2>&1"}, "/etc/crontab", job.System, now)
	if j.Name != "backup" || j.LogPath != "/var/log/b-" {
		t.Errorf("name = %q, log = %q", j.Name, j.LogPath)
	}
	if j.Command != "/usr/bin/backup >> /var/log/b-%F.log 2>&1" {
		t.Errorf("the full command should be kept: %q", j.Command)
	}
}

func TestUnreadable(t *testing.T) {
	j := unreadable("/etc/cron.d/secret", &fs.PathError{Op: "open", Path: "/etc/cron.d/secret", Err: fs.ErrPermission})
	if !j.Unreadable || j.Err != "permission denied" || j.Name != "secret" {
		t.Errorf("%+v", j)
	}
	if j = unreadable("/etc/crontab", errors.New("boom")); j.Err != "boom" {
		t.Errorf("err = %q", j.Err)
	}
}

func TestFindRunner(t *testing.T) {
	debian := []job.Job{
		{Expr: "17 * * * *", Command: "cd / && run-parts --report /etc/cron.hourly"},
		{Expr: "25 6 * * *", Command: "test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.daily; }"},
	}
	tests := []struct {
		dir     string
		anacron bool
		want    runner
	}{
		{"/etc/cron.hourly", true, runner{expr: "17 * * * *"}},
		{"/etc/cron.daily", false, runner{expr: "25 6 * * *"}},
		{"/etc/cron.daily", true, runner{anacron: true}},
		{"/etc/cron.weekly", true, runner{anacron: true}},
		{"/etc/cron.weekly", false, runner{}},
	}
	for _, tt := range tests {
		if got := findRunner(debian, tt.dir, tt.anacron); got != tt.want {
			t.Errorf("findRunner(%s, anacron=%v) = %+v, want %+v", tt.dir, tt.anacron, got, tt.want)
		}
	}
}
