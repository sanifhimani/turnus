package launchd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"howett.net/plist"

	"github.com/sanifhimani/turnus/internal/schedule"
)

// spec holds the plist keys that affect when and how a job runs.
type spec struct {
	Label                 string   `plist:"Label"`
	Program               string   `plist:"Program"`
	ProgramArguments      []string `plist:"ProgramArguments"`
	Disabled              bool     `plist:"Disabled"`
	RunAtLoad             bool     `plist:"RunAtLoad"`
	KeepAlive             any      `plist:"KeepAlive"`
	StartInterval         int      `plist:"StartInterval"`
	StartCalendarInterval any      `plist:"StartCalendarInterval"`
	StartOnMount          bool     `plist:"StartOnMount"`
	WatchPaths            []string `plist:"WatchPaths"`
	QueueDirectories      []string `plist:"QueueDirectories"`
	StandardOutPath       string   `plist:"StandardOutPath"`
	StandardErrorPath     string   `plist:"StandardErrorPath"`
}

func readSpec(path string) (spec, error) {
	var s spec
	data, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}
	_, err = plist.Unmarshal(data, &s)
	return s, err
}

func (s spec) program() string {
	if s.Program != "" {
		return s.Program
	}
	if len(s.ProgramArguments) > 0 {
		return s.ProgramArguments[0]
	}
	return ""
}

func (s spec) command() string {
	args := s.ProgramArguments
	if len(args) == 0 && s.Program != "" {
		args = []string{s.Program}
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = shellQuote(a)
	}
	return strings.Join(quoted, " ")
}

// keepAlive reports whether launchd restarts the job when it exits. A dict
// value means "under some conditions", which still counts.
func (s spec) keepAlive() bool {
	switch v := s.KeepAlive.(type) {
	case bool:
		return v
	case map[string]any:
		return len(v) > 0
	}
	return false
}

// calendar converts StartCalendarInterval, a dict or an array of dicts, to
// cron expressions. Missing keys are wildcards, as they are in launchd.
func (s spec) calendar() ([]string, error) {
	var dicts []any
	switch v := s.StartCalendarInterval.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		dicts = []any{v}
	case []any:
		dicts = v
	default:
		return nil, fmt.Errorf("StartCalendarInterval has type %T", v)
	}

	exprs := make([]string, 0, len(dicts))
	for _, d := range dicts {
		m, ok := d.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("StartCalendarInterval entry has type %T", d)
		}
		fields := make([]string, 5)
		for i, key := range []string{"Minute", "Hour", "Day", "Month", "Weekday"} {
			fields[i] = "*"
			if v, ok := m[key]; ok {
				n, ok := toInt(v)
				if !ok {
					return nil, fmt.Errorf("%s has type %T", key, v)
				}
				fields[i] = fmt.Sprint(n)
			}
		}
		exprs = append(exprs, strings.Join(fields, " "))
	}
	return exprs, nil
}

// trigger is what makes launchd start a job.
type trigger struct {
	desc      string
	expr      string
	next      time.Time
	scheduled bool
}

func (s spec) trigger(daemon bool, now time.Time) trigger {
	exprs, err := s.calendar()
	switch {
	case err != nil:
		return trigger{desc: "invalid calendar", scheduled: true}
	case len(exprs) > 0:
		return calendarTrigger(exprs, now)
	case s.StartInterval > 0:
		d := time.Duration(s.StartInterval) * time.Second
		return trigger{desc: schedule.Every(d), expr: fmt.Sprintf("StartInterval %d", s.StartInterval), scheduled: true}
	case len(s.WatchPaths) > 0:
		return trigger{desc: "when " + filepath.Base(s.WatchPaths[0]) + " changes", scheduled: true}
	case len(s.QueueDirectories) > 0:
		return trigger{desc: "when files land in " + filepath.Base(s.QueueDirectories[0]), scheduled: true}
	case s.StartOnMount:
		return trigger{desc: "when a disk mounts", scheduled: true}
	case s.keepAlive():
		return trigger{desc: "always running"}
	case s.RunAtLoad && daemon:
		return trigger{desc: "at boot"}
	case s.RunAtLoad:
		return trigger{desc: "at login"}
	}
	return trigger{desc: "on demand"}
}

func calendarTrigger(exprs []string, now time.Time) trigger {
	t := trigger{expr: strings.Join(exprs, ", "), scheduled: true}
	if merged, ok := mergeCron(exprs); ok {
		exprs = []string{merged}
	}
	var descs []string
	for _, e := range exprs {
		c, err := schedule.ParseCron(e)
		if err != nil {
			t.desc = "invalid calendar"
			return t
		}
		if n := c.Next(now); !n.IsZero() && (t.next.IsZero() || n.Before(t.next)) {
			t.next = n
		}
		d := c.Describe()
		if d == "" {
			d = e
		}
		if !slices.Contains(descs, d) {
			descs = append(descs, d)
		}
	}

	t.desc = descs[0]
	if len(descs) > 1 {
		t.desc = fmt.Sprintf("%d schedules", len(exprs))
	}
	return t
}

// mergeCron combines expressions that differ in a single field, so that
// Monday 9:00 and Wednesday 9:00 become "0 9 * * 1,3".
func mergeCron(exprs []string) (string, bool) {
	if len(exprs) < 2 {
		return "", false
	}
	base := strings.Fields(exprs[0])
	diff := -1
	for _, e := range exprs[1:] {
		f := strings.Fields(e)
		for i := range f {
			if f[i] == base[i] {
				continue
			}
			if diff != -1 && diff != i {
				return "", false
			}
			diff = i
		}
	}
	if diff == -1 {
		return exprs[0], true
	}
	values := make([]string, 0, len(exprs))
	for _, e := range exprs {
		v := strings.Fields(e)[diff]
		if v == "*" {
			return "", false
		}
		values = append(values, v)
	}
	base[diff] = strings.Join(values, ",")
	return strings.Join(base, " "), true
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int64:
		return int(n), true
	case uint64:
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}

func shellQuote(s string) string {
	if s != "" && !strings.ContainsAny(s, " \t\n'\"\\$`*?[]{}()<>|&;#~") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
