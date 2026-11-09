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

func TestUnreadable(t *testing.T) {
	j := unreadable("/etc/cron.d/secret", &fs.PathError{Op: "open", Path: "/etc/cron.d/secret", Err: fs.ErrPermission})
	if !j.Unreadable || j.Err != "permission denied" || j.Name != "secret" {
		t.Errorf("%+v", j)
	}
	if j = unreadable("/etc/crontab", errors.New("boom")); j.Err != "boom" {
		t.Errorf("err = %q", j.Err)
	}
}
