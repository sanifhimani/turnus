package crontab

import (
	"slices"
	"testing"
)

func TestCommandName(t *testing.T) {
	tests := []struct {
		cmd, name, program string
		interpreted        bool
	}{
		{"/usr/local/bin/backup.sh >> /var/log/b.log 2>&1", "backup.sh", "/usr/local/bin/backup.sh", false},
		{"cd /srv/app && ./bin/sync --fast", "sync", "/srv/app/bin/sync", false},
		{"/usr/bin/python3 /opt/jobs/report.py --daily", "report.py", "/opt/jobs/report.py", true},
		{"php artisan schedule:run", "artisan", "", true},
		{"FOO=1 BAR=2 /usr/bin/thing", "thing", "/usr/bin/thing", false},
		{"bash -c 'echo hi'", "bash", "", false},
		{"curl -fsS https://example.com/ping", "curl", "", false},
		{`"/Users/me/Library/Application Support/x/run.sh" --now`, "run.sh", "/Users/me/Library/Application Support/x/run.sh", false},
		{`/opt/My\ Scripts/sync.sh`, "sync.sh", "/opt/My Scripts/sync.sh", false},
		{"test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.daily; }", "cron.daily", "", false},
		{"cd / && run-parts --report /etc/cron.hourly", "cron.hourly", "", false},
		{"[ -x /usr/bin/thing ] && /usr/bin/thing --go", "thing", "/usr/bin/thing", false},
	}
	for _, tt := range tests {
		name, program, interpreted := commandName(tt.cmd)
		if name != tt.name || program != tt.program || interpreted != tt.interpreted {
			t.Errorf("commandName(%q) = %q, %q, %v; want %q, %q, %v", tt.cmd, name, program, interpreted, tt.name, tt.program, tt.interpreted)
		}
	}
}

func TestLogPath(t *testing.T) {
	tests := []struct{ cmd, want string }{
		{"/bin/x >> /var/log/x.log 2>&1", "/var/log/x.log"},
		{"/bin/x >/tmp/out", "/tmp/out"},
		{"/bin/x > /dev/null 2>&1", ""},
		{"/bin/x 2>&1", ""},
		{"/bin/x", ""},
	}
	for _, tt := range tests {
		if got := logPath(tt.cmd); got != tt.want {
			t.Errorf("logPath(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestShellFields(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"a b  c", []string{"a", "b", "c"}},
		{`echo "two  spaces"`, []string{"echo", "two  spaces"}},
		{`echo 'it''s'`, []string{"echo", "its"}},
		{`echo a\ b`, []string{"echo", "a b"}},
		{`echo ""`, []string{"echo", ""}},
		{`x >> "/tmp/my log"`, []string{"x", ">>", "/tmp/my log"}},
		{"", nil},
	}
	for _, tt := range tests {
		if got := shellFields(tt.in); !slices.Equal(got, tt.want) {
			t.Errorf("shellFields(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestUnescapedPercent(t *testing.T) {
	tests := []struct {
		cmd  string
		want int
	}{
		{"tar czf /backup/$(date +%F).tgz /srv", 24},
		{`tar czf /backup/$(date +\%F).tgz /srv`, -1},
		{"echo done", -1},
		{`printf '\\%s' x`, 10},
	}
	for _, tt := range tests {
		if got := UnescapedPercent(tt.cmd); got != tt.want {
			t.Errorf("UnescapedPercent(%q) = %d, want %d", tt.cmd, got, tt.want)
		}
	}
}
