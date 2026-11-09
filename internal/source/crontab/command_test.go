package crontab

import (
	"slices"
	"testing"
)

func TestCommandName(t *testing.T) {
	tests := []struct{ cmd, name, program string }{
		{"/usr/local/bin/backup.sh >> /var/log/b.log 2>&1", "backup.sh", "/usr/local/bin/backup.sh"},
		{"cd /srv/app && ./bin/sync --fast", "sync", "/srv/app/bin/sync"},
		{"/usr/bin/python3 /opt/jobs/report.py --daily", "report.py", "/opt/jobs/report.py"},
		{"php artisan schedule:run", "artisan", ""},
		{"FOO=1 BAR=2 /usr/bin/thing", "thing", "/usr/bin/thing"},
		{"bash -c 'echo hi'", "bash", ""},
		{"curl -fsS https://example.com/ping", "curl", ""},
		{`"/Users/me/Library/Application Support/x/run.sh" --now`, "run.sh", "/Users/me/Library/Application Support/x/run.sh"},
		{`/opt/My\ Scripts/sync.sh`, "sync.sh", "/opt/My Scripts/sync.sh"},
	}
	for _, tt := range tests {
		name, program := commandName(tt.cmd)
		if name != tt.name || program != tt.program {
			t.Errorf("commandName(%q) = %q, %q; want %q, %q", tt.cmd, name, program, tt.name, tt.program)
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
