package crontab

import (
	"testing"

	"github.com/sanifhimani/turnus/internal/job"
)

func TestPackageFiles(t *testing.T) {
	files := packageFiles("testdata/dpkg-status")
	for _, want := range []string{"/etc/cron.daily/dpkg", "/etc/cron.d/e2scrub_all", "/etc/crontab"} {
		if !files[want] {
			t.Errorf("missing %s", want)
		}
	}
	if files["/etc/dpkg/dpkg.cfg"] {
		t.Error("non-cron files should be skipped")
	}
	if len(packageFiles("testdata/missing")) != 0 {
		t.Error("a missing database should give no files")
	}
}

func TestMarkBuiltin(t *testing.T) {
	jobs := []job.Job{
		{File: "/etc/crontab", Command: "cd / && run-parts --report /etc/cron.hourly"},
		{File: "/etc/crontab", Command: "/usr/local/bin/backup"},
		{File: "/etc/cron.daily/dpkg"},
		{File: "/etc/cron.d/myapp"},
		{File: "/var/spool/cron/crontabs/root"},
	}
	markBuiltin(jobs, packageFiles("testdata/dpkg-status"))
	want := []bool{true, false, true, false, false}
	for i, j := range jobs {
		if j.Builtin != want[i] {
			t.Errorf("%s %q: builtin = %v, want %v", j.File, j.Command, j.Builtin, want[i])
		}
	}
}
