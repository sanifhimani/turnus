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

func TestPathLookups(t *testing.T) {
	tests := []struct {
		cmd  string
		want []string
	}{
		{"node /opt/sync.js", []string{"node"}},
		{"cd /srv && docker compose up -d", []string{"docker"}},
		{"pg_dump app | gzip > /backups/app.gz", []string{"pg_dump", "gzip"}},
		{"FOO=1 BAR=2 rsync -a /src /dst; rsync -a /x /y", []string{"rsync"}},
		{"if [ -f /tmp/go ]; then exec python3 /opt/run.py; fi", []string{"python3"}},
		{"nohup node /srv/app.js &", []string{"node"}},
		{"test -x /usr/sbin/anacron || { cd / && run-parts --report /etc/cron.daily; }", []string{"run-parts"}},
		{"/usr/local/bin/backup.sh >> /var/log/b.log 2>&1", nil},
		{"cd /srv && ./bin/sync", nil},
		{"$HOME/bin/sync && ~/bin/other", nil},
		{"echo hi && true", nil},
		{"find . -name '*.tmp' -delete", []string{"find"}},

		// The command may change PATH before the lookups happen.
		{"PATH=/opt/homebrew/bin:$PATH node /opt/sync.js", nil},
		{"export PATH=/opt/bin:/usr/bin; node x.js", nil},
		{". $HOME/.profile && node /opt/sync.js", nil},
		{"source ~/.nvm/nvm.sh; node /opt/sync.js", nil},
		{"bash -lc 'node /opt/sync.js'", nil},
		{"/bin/bash --login -c 'node /opt/sync.js'", nil},
		{"zsh -c 'node /opt/sync.js'", nil},
		{"/opt/homebrew/bin/fish -c 'node /opt/sync.js'", nil},
		{"bash -c 'node /opt/sync.js'", []string{"bash", "node"}},
	}
	for _, tt := range tests {
		if got := pathLookups(tt.cmd); !slices.Equal(got, tt.want) {
			t.Errorf("pathLookups(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestAssignment(t *testing.T) {
	for word, want := range map[string]bool{
		"FOO=1": true, "_x=": true, "A1=b": true,
		"1A=b": false, "=x": false, "--opt=1": false, "a-b=1": false, "node": false,
	} {
		if got := assignment(word); got != want {
			t.Errorf("assignment(%q) = %v, want %v", word, got, want)
		}
	}
}
