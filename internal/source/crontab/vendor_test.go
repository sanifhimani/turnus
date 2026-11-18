package crontab

import (
	"slices"
	"testing"

	"github.com/sanifhimani/turnus/internal/job"
)

func TestPackageFiles(t *testing.T) {
	files := dpkgFiles("testdata/dpkg-status")
	for _, want := range []string{"/etc/cron.daily/dpkg", "/etc/cron.d/e2scrub_all", "/etc/crontab"} {
		if !files[want] {
			t.Errorf("missing %s", want)
		}
	}
	if files["/etc/dpkg/dpkg.cfg"] {
		t.Error("non-cron files should be skipped")
	}
	if len(dpkgFiles("testdata/missing")) != 0 {
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
	markBuiltin(jobs, dpkgFiles("testdata/dpkg-status"))
	want := []bool{true, false, true, false, false}
	for i, j := range jobs {
		if j.Builtin != want[i] {
			t.Errorf("%s %q: builtin = %v, want %v", j.File, j.Command, j.Builtin, want[i])
		}
	}
}

func TestCronFiles(t *testing.T) {
	jobs := []job.Job{
		{File: "/etc/crontab"},
		{File: "/etc/cron.d/0hourly"},
		{File: "/etc/cron.d/0hourly"},
		{File: "/etc/cron.daily/logrotate"},
		{File: "/var/spool/cron/root"},
		{File: "/usr/lib/cron/tabs/me"},
	}
	got := cronFiles(jobs)
	want := []string{"/etc/cron.d/0hourly", "/etc/cron.daily/logrotate"}
	if !slices.Equal(got, want) {
		t.Errorf("cronFiles = %q, want %q", got, want)
	}
}

// rpm -qf --qf '[%{FILENAMES}\n]' lists every file of the owning packages,
// and a "not owned" line for files no package owns.
func TestListed(t *testing.T) {
	out := []byte(`/etc/cron.d
/etc/cron.d/0hourly
/etc/cron.deny
/usr/sbin/crond
file /etc/cron.d/myapp is not owned by any package
`)
	got := listed(out, []string{"/etc/cron.d/0hourly", "/etc/cron.d/myapp"})
	if !got["/etc/cron.d/0hourly"] || got["/etc/cron.d/myapp"] || len(got) != 1 {
		t.Errorf("listed = %v", got)
	}
}

func TestApkOwned(t *testing.T) {
	out := []byte(`/etc/cron.d/backup is owned by backup-tool-1.2-r0
`)
	got := apkOwned(out)
	if !got["/etc/cron.d/backup"] || len(got) != 1 {
		t.Errorf("apkOwned = %v", got)
	}
}
