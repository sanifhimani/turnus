package crontab

import (
	"strings"
	"testing"
)

func TestParseUser(t *testing.T) {
	const tab = `# backups
MAILTO=ops@example.com
SHELL=/bin/bash

0 3 * * * /usr/local/bin/backup.sh >> /var/log/backup.log 2>&1
*/15  9-17 * * 1-5   cd /srv/app && ./bin/sync   --fast
@reboot /opt/start.sh
MAILTO=""
@daily   echo  "two  spaces"
not a valid line
`
	entries, err := Parse(strings.NewReader(tab), false)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Line: 5, Expr: "0 3 * * *", Command: "/usr/local/bin/backup.sh >> /var/log/backup.log 2>&1"},
		{Line: 6, Expr: "*/15 9-17 * * 1-5", Command: "cd /srv/app && ./bin/sync   --fast"},
		{Line: 7, Expr: "@reboot", Command: "/opt/start.sh"},
		{Line: 9, Expr: "@daily", Command: `echo  "two  spaces"`},
		{Line: 10, Command: "not a valid line", Err: "it needs a schedule and a command"},
	}
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i, w := range want {
		g := entries[i]
		if g.Line != w.Line || g.Expr != w.Expr || g.Command != w.Command || g.Err != w.Err || g.User != "" {
			t.Errorf("entry %d = %+v, want %+v", i, g, w)
		}
	}

	if got := entries[0].Env["MAILTO"]; got != "ops@example.com" {
		t.Errorf("MAILTO before reset = %q", got)
	}
	if got, ok := entries[3].Env["MAILTO"]; !ok || got != "" {
		t.Errorf("MAILTO after reset = %q, %v", got, ok)
	}
}

func TestParseSystem(t *testing.T) {
	const tab = `SHELL=/bin/sh
17 *	* * *	root	cd / && run-parts --report /etc/cron.hourly
@weekly backup /usr/bin/rotate
`
	entries, err := Parse(strings.NewReader(tab), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if e := entries[0]; e.Expr != "17 * * * *" || e.User != "root" || e.Command != "cd / && run-parts --report /etc/cron.hourly" {
		t.Errorf("entry 0 = %+v", e)
	}
	if e := entries[1]; e.Expr != "@weekly" || e.User != "backup" || e.Command != "/usr/bin/rotate" {
		t.Errorf("entry 1 = %+v", e)
	}
}

func TestParseMissingCommand(t *testing.T) {
	entries, _ := Parse(strings.NewReader("* * * * *\n0 3 * * * root\n"), true)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	for _, e := range entries {
		if e.Err != "it needs a schedule, a user and a command" {
			t.Errorf("line %d: err = %q", e.Line, e.Err)
		}
	}
}
