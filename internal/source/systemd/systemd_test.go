package systemd

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

func loadFixture(t *testing.T) map[string]unit {
	t.Helper()
	f, err := os.Open("testdata/show.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	return parseShow(f)
}

func TestParseShow(t *testing.T) {
	units := loadFixture(t)
	if len(units) != 22 {
		t.Fatalf("got %d units, want 22", len(units))
	}
	if got := len(units["lab-interval.timer"]["TimersMonotonic"]); got != 2 {
		t.Errorf("interval timer has %d monotonic triggers, want 2", got)
	}
}

func TestToJob(t *testing.T) {
	units := loadFixture(t)
	tests := []struct {
		timer    string
		schedule string
		command  string
		state    job.State
		exit     *int
		err      bool
	}{
		{timer: "lab-ok.timer", schedule: "every 5 minutes", command: "/bin/true", state: job.StateIdle, exit: ptr(0)},
		{timer: "lab-fail.timer", schedule: "every 10 minutes from :02", command: "/bin/sh -c echo failing on purpose >&2; exit 3", state: job.StateIdle, exit: ptr(3)},
		{timer: "lab-crash.timer", schedule: "hourly", command: "/usr/local/bin/lab-crash", state: job.StateIdle, exit: ptr(-11)},
		{timer: "lab-missing.timer", schedule: "daily at 00:00", command: "/usr/local/bin/turnus-lab-missing --flag", state: job.StateIdle, exit: ptr(203)},
		{timer: "lab-persist.timer", schedule: "weekdays at 09:00", command: "/bin/true", state: job.StateIdle},
		{timer: "lab-interval.timer", schedule: "hourly · 15 minutes after boot", command: "/bin/true", state: job.StateIdle},
		{timer: "lab-disabled.timer", schedule: "Mondays at 00:00", command: "/bin/true", state: job.StateDisabled},
		{timer: "lab-stopped.timer", schedule: "monthly on the 1st at 00:00", command: "/bin/true", state: job.StateUnloaded},
		{timer: "lab-orphan.timer", schedule: "daily at 00:00", state: job.StateUnloaded, err: true},
	}
	for _, tt := range tests {
		t.Run(tt.timer, func(t *testing.T) {
			timer := units[tt.timer]
			j := toJob(timer, units[timer.get("Unit")], job.System)
			if j.Schedule != tt.schedule || j.Command != tt.command || j.State != tt.state {
				t.Errorf("schedule=%q command=%q state=%q", j.Schedule, j.Command, j.State)
			}
			if (j.LastExit == nil) != (tt.exit == nil) || (j.LastExit != nil && *j.LastExit != *tt.exit) {
				t.Errorf("last exit = %v, want %v", deref(j.LastExit), deref(tt.exit))
			}
			if (j.Err != "") != tt.err {
				t.Errorf("err = %q", j.Err)
			}
		})
	}

	persist := toJob(units["lab-persist.timer"], units["lab-persist.service"], job.System)
	if !persist.Persistent || persist.Next.IsZero() || persist.Builtin {
		t.Errorf("persist: persistent=%v next=%v builtin=%v", persist.Persistent, persist.Next, persist.Builtin)
	}
}

func ptr(n int) *int { return &n }

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestTimerNames(t *testing.T) {
	data, err := os.ReadFile("testdata/list-units.txt")
	if err != nil {
		t.Fatal(err)
	}
	names := timerNames(data)
	if !slices.Contains(names, "lab-ok.timer") || !slices.Contains(names, "apt-daily.timer") || slices.Contains(names, "lab-disabled.timer") {
		t.Errorf("got %d names: %v", len(names), names)
	}
	if got := timerNames([]byte("foo@.timer loaded inactive dead x\n")); len(got) != 0 {
		t.Errorf("template timers should be skipped, got %v", got)
	}
}

func TestField(t *testing.T) {
	const v = "{ path=/bin/sh ; argv[]=/bin/sh -c a; b ; ignore_errors=no ; start_time=[n/a] }"
	if got := field(v, "path", "argv[]"); got != "/bin/sh" {
		t.Errorf("path = %q", got)
	}
	if got := field(v, "argv[]", "ignore_errors"); got != "/bin/sh -c a; b" {
		t.Errorf("argv = %q", got)
	}
	if got := field("{ OnCalendar=*-*-* 02:30:00 ; next_elapse=(null) }", "OnCalendar", "next_elapse"); got != "*-*-* 02:30:00" {
		t.Errorf("calendar = %q", got)
	}
}

func TestDescribeMonotonic(t *testing.T) {
	tests := []struct{ key, span, want string }{
		{"OnUnitActiveUSec", "1h", "hourly"},
		{"OnUnitInactiveUSec", "30min", "every 30 minutes"},
		{"OnBootUSec", "15min", "15 minutes after boot"},
		{"OnBootUSec", "1h", "1 hour after boot"},
		{"OnStartupUSec", "90s", "90 seconds after startup"},
		{"OnActiveUSec", "1d", "1 day after the timer starts"},
		{"OnUnitActiveUSec", "bogus", "OnUnitActiveUSec=bogus"},
	}
	for _, tt := range tests {
		if got := describeMonotonic(tt.key, tt.span); got != tt.want {
			t.Errorf("describeMonotonic(%q, %q) = %q, want %q", tt.key, tt.span, got, tt.want)
		}
	}
}

func TestParseTimespan(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
		ok   bool
	}{
		{"15min", 15 * time.Minute, true},
		{"1h 30min", 90 * time.Minute, true},
		{"1d 2h", 26 * time.Hour, true},
		{"2.5s", 2500 * time.Millisecond, true},
		{"1w", 7 * 24 * time.Hour, true},
		{"", 0, false},
		{"5 apples", 0, false},
	}
	for _, tt := range tests {
		got, ok := parseTimespan(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("parseTimespan(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestVendor(t *testing.T) {
	if !vendor("/usr/lib/systemd/system/apt-daily.timer") || !vendor("/lib/systemd/system/x.timer") {
		t.Error("distribution units should be vendor units")
	}
	if vendor("/etc/systemd/system/backup.timer") || vendor("/home/me/.config/systemd/user/x.timer") {
		t.Error("admin and user units are not vendor units")
	}
}

func TestBooted(t *testing.T) {
	dir := t.TempDir()
	if !booted(dir) {
		t.Error("an existing marker folder means systemd is running")
	}
	if booted(dir + "/missing") {
		t.Error("a missing marker means systemd isn't running")
	}
}
