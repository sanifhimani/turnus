package crontab

import (
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
