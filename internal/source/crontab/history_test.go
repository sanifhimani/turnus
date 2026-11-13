package crontab

import (
	"strings"
	"testing"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

func TestParseSyslog(t *testing.T) {
	now := time.Date(2026, 11, 13, 18, 0, 0, 0, time.UTC)

	// Debian 12's rsyslog, with cron started normally and with -L 15.
	debian := `2026-11-13T17:03:35.432665+00:00 host cron[3525]: (CRON) INFO (pidfile fd = 3)
2026-11-13T17:04:01.439563+00:00 host CRON[3534]: (root) CMD (echo boom >&2; exit 3)
2026-11-13T17:04:01.439705+00:00 host systemd[1]: Started something.
2026-11-13T17:05:01.484890+00:00 host CRON[3532]: (root) CMD ([3533] /usr/bin/backup --full)
2026-11-13T17:05:01.485701+00:00 host CRON[3530]: (CRON) error (grandchild #3536 failed with exit status 3)
2026-11-13T17:05:01.486136+00:00 host CRON[3531]: (root) END ([3534] /bin/false)
`
	h, ok := parseSyslog(strings.NewReader(debian), now)
	if !ok || !h.since.Equal(time.Date(2026, 11, 13, 17, 3, 35, 432665000, time.UTC)) {
		t.Fatalf("since = %v, ok = %v", h.since, ok)
	}
	want := []run{
		{"root", "echo boom >&2; exit 3", time.Date(2026, 11, 13, 17, 4, 1, 439563000, time.UTC)},
		{"root", "/usr/bin/backup --full", time.Date(2026, 11, 13, 17, 5, 1, 484890000, time.UTC)},
	}
	if len(h.runs) != len(want) {
		t.Fatalf("runs = %+v", h.runs)
	}
	for i := range want {
		if h.runs[i].user != want[i].user || h.runs[i].cmd != want[i].cmd || !h.runs[i].at.Equal(want[i].at) {
			t.Errorf("run %d = %+v, want %+v", i, h.runs[i], want[i])
		}
	}

	// cronie's /var/log/cron, with traditional timestamps.
	cronie := `Nov  3 09:00:01 host crond[812]: (CRON) STARTUP (1.7.2)
Nov 13 17:04:01 host CROND[116]: (me) CMD (/home/me/sync.sh)
Nov 13 17:04:01 host CROND[116]: (me) CMDOUT (synced)
Nov 13 17:04:02 host CROND[116]: (me) CMDEND (/home/me/sync.sh)
`
	h, ok = parseSyslog(strings.NewReader(cronie), now)
	if !ok || !h.since.Equal(time.Date(2026, 11, 3, 9, 0, 1, 0, time.UTC)) {
		t.Fatalf("cronie since = %v, ok = %v", h.since, ok)
	}
	if len(h.runs) != 1 || h.runs[0].user != "me" || h.runs[0].cmd != "/home/me/sync.sh" {
		t.Errorf("cronie runs = %+v", h.runs)
	}

	if _, ok := parseSyslog(strings.NewReader("not a log\n"), now); ok {
		t.Error("a file without timestamps should give no history")
	}
}

func TestSyslogTimeYear(t *testing.T) {
	// A December line read on January 2 is from last year.
	now := time.Date(2027, 1, 2, 12, 0, 0, 0, time.UTC)
	at, _, ok := syslogTime("Dec 31 23:59:01 host CRON[1]: (root) CMD (x)", now)
	if !ok || at.Year() != 2026 {
		t.Errorf("at = %v, ok = %v", at, ok)
	}
	at, _, _ = syslogTime("Jan  2 08:00:00 host CRON[1]: (root) CMD (x)", now)
	if at.Year() != 2027 || at.Day() != 2 {
		t.Errorf("at = %v", at)
	}
}

func TestParseJournal(t *testing.T) {
	out := `{"__REALTIME_TIMESTAMP":"1794900000000000","MESSAGE":"(CRON) INFO (Running @reboot jobs)"}
{"__REALTIME_TIMESTAMP":"1794903600000000","MESSAGE":"(root) CMD (/usr/bin/backup)"}
{"__REALTIME_TIMESTAMP":"1794903660000000","MESSAGE":[40,114,111,111,116,41]}
{"__REALTIME_TIMESTAMP":"1794907200000000","MESSAGE":"(me) CMD ([42] node /srv/sync.js)"}
`
	h, ok := parseJournal(strings.NewReader(out))
	if !ok || !h.since.Equal(time.UnixMicro(1794900000000000)) {
		t.Fatalf("since = %v, ok = %v", h.since, ok)
	}
	if len(h.runs) != 2 || h.runs[1].user != "me" || h.runs[1].cmd != "node /srv/sync.js" {
		t.Errorf("runs = %+v", h.runs)
	}
	if _, ok := parseJournal(strings.NewReader("")); ok {
		t.Error("an empty journal should give no history")
	}
}

func TestApplyHistory(t *testing.T) {
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	at := func(d int) time.Time { return time.Date(2026, 11, d, 3, 0, 0, 0, time.UTC) }
	h := history{since: since, runs: []run{
		{"root", "/usr/bin/backup", at(10)},
		{"root", "/usr/bin/backup", at(12)},
		{"me", "/usr/bin/backup", at(13)},
		{"root", "test -x /usr/sbin/anacron || run-parts /etc/cron.daily", at(13)},
	}}
	jobs := []job.Job{
		{Source: job.Cron, Command: "/usr/bin/backup", RunAs: "root"},
		{Source: job.Cron, Command: "/usr/bin/backup"},
		{Source: job.Cron, Command: "/usr/bin/other", RunAs: "root"},
		{Source: job.Cron, Command: "/etc/cron.daily/logrotate", File: "/etc/cron.daily/logrotate"},
		{Source: job.Launchd, Command: "/usr/bin/backup"},
	}
	applyHistory(jobs, h)

	if !jobs[0].LastRun.Equal(at(12)) || !jobs[0].HistorySince.Equal(since) {
		t.Errorf("root's job: last run %v, since %v", jobs[0].LastRun, jobs[0].HistorySince)
	}
	if !jobs[1].LastRun.Equal(at(13)) {
		t.Errorf("job with no known user should match any user: %v", jobs[1].LastRun)
	}
	if !jobs[2].LastRun.IsZero() || !jobs[2].HistorySince.Equal(since) {
		t.Errorf("job that never ran: last run %v, since %v", jobs[2].LastRun, jobs[2].HistorySince)
	}
	if !jobs[3].HistorySince.IsZero() {
		t.Error("periodic scripts have no line of their own, so their history is unknown")
	}
	if !jobs[4].LastRun.IsZero() || !jobs[4].HistorySince.IsZero() {
		t.Error("launchd jobs aren't in cron's log")
	}
}
