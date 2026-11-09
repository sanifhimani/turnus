// Package systemd finds jobs defined as systemd timers on Linux.
package systemd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

var timerProps = []string{
	"Id", "Unit", "FragmentPath", "LoadState", "ActiveState", "UnitFileState",
	"TimersCalendar", "TimersMonotonic", "NextElapseUSecRealtime", "Persistent",
}

var serviceProps = []string{
	"Id", "LoadState", "ActiveState", "ExecStart", "ExecMainCode", "ExecMainStatus", "MainPID",
}

// Discover returns timers from the system manager and the current user's
// manager. Timers shipped by the distribution are included only when
// builtin is true.
func Discover(ctx context.Context, now time.Time, builtin bool) ([]job.Job, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return nil, nil
	}

	var (
		wg           sync.WaitGroup
		system, user []job.Job
		err          error
	)
	wg.Go(func() { system, err = discoverScope(ctx, job.System, builtin) })
	// The user manager is often absent, such as over SSH without lingering
	// or in containers, so its errors are not reported.
	wg.Go(func() { user, _ = discoverScope(ctx, job.User, builtin) })
	wg.Wait()
	return slices.Concat(system, user), err
}

func discoverScope(ctx context.Context, scope job.Scope, builtin bool) ([]job.Job, error) {
	// list-unit-files reads every unit on disk and takes about 100ms, so
	// look for timer files directly and add timers that are only loaded,
	// such as generated ones.
	out, err := systemctl(ctx, scope, "list-units", "--type=timer", "--all", "--plain", "--no-legend", "--no-pager")
	if err != nil {
		return nil, err
	}
	timers := timerNames(out)
	for _, dir := range unitDirs(scope) {
		paths, _ := filepath.Glob(filepath.Join(dir, "*.timer"))
		for _, p := range paths {
			if name := filepath.Base(p); !strings.HasSuffix(name, "@.timer") && !slices.Contains(timers, name) {
				timers = append(timers, name)
			}
		}
	}
	if len(timers) == 0 {
		return nil, nil
	}
	slices.Sort(timers)

	// Ask for each timer and the service it most likely starts in one call.
	names := slices.Clone(timers)
	for _, t := range timers {
		names = append(names, strings.TrimSuffix(t, ".timer")+".service")
	}
	units, err := show(ctx, scope, names, append(timerProps, serviceProps...))
	if err != nil {
		return nil, err
	}

	// Timers can start a differently named unit with Unit=.
	var missing []string
	for _, t := range timers {
		if svc := units[t].get("Unit"); svc != "" && units[svc] == nil {
			missing = append(missing, svc)
		}
	}
	if len(missing) > 0 {
		more, err := show(ctx, scope, missing, serviceProps)
		if err != nil {
			return nil, err
		}
		for id, u := range more {
			units[id] = u
		}
	}

	jobs := make([]job.Job, 0, len(timers))
	for _, t := range timers {
		timer := units[t]
		if timer == nil {
			continue
		}
		j := toJob(timer, units[timer.get("Unit")], scope)
		if j.Builtin && !builtin {
			continue
		}
		jobs = append(jobs, j)
	}
	return jobs, nil
}

// unitDirs are the folders systemd loads unit files from, as listed in
// systemd.unit(5).
func unitDirs(scope job.Scope) []string {
	if scope == job.User {
		home, _ := os.UserHomeDir()
		return []string{
			filepath.Join(home, ".config/systemd/user"),
			"/etc/systemd/user",
			filepath.Join(home, ".local/share/systemd/user"),
			"/usr/local/lib/systemd/user",
			"/usr/lib/systemd/user",
		}
	}
	return []string{
		"/etc/systemd/system",
		"/run/systemd/system",
		"/usr/local/lib/systemd/system",
		"/usr/lib/systemd/system",
		"/lib/systemd/system",
	}
}

// timerNames reads the unit names from systemctl listings.
func timerNames(listing []byte) []string {
	var names []string
	sc := bufio.NewScanner(bytes.NewReader(listing))
	for sc.Scan() {
		if f := strings.Fields(sc.Text()); len(f) > 0 && strings.HasSuffix(f[0], ".timer") {
			// Template units like foo@.timer only run as instances.
			if !strings.HasSuffix(f[0], "@.timer") {
				names = append(names, f[0])
			}
		}
	}
	return names
}

func show(ctx context.Context, scope job.Scope, names, props []string) (map[string]unit, error) {
	args := append([]string{"show", "--no-pager", "-p", strings.Join(props, ",")}, names...)
	out, err := systemctl(ctx, scope, args...)
	if err != nil {
		return nil, err
	}
	return parseShow(bytes.NewReader(out)), nil
}

func systemctl(ctx context.Context, scope job.Scope, args ...string) ([]byte, error) {
	if scope == job.User {
		args = append([]string{"--user"}, args...)
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "systemctl", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("systemctl %s: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func toJob(timer, svc unit, scope job.Scope) job.Job {
	id := timer.get("Id")
	j := job.Job{
		ID:         "systemd:" + string(scope) + ":" + id,
		Name:       strings.TrimSuffix(id, ".timer"),
		Source:     job.Systemd,
		Scope:      scope,
		Kind:       job.Scheduled,
		Builtin:    vendor(timer.get("FragmentPath")),
		File:       timer.get("FragmentPath"),
		Unit:       timer.get("Unit"),
		Persistent: timer.get("Persistent") == "yes",
	}

	j.Schedule, j.Expr = trigger(timer)
	j.Next = parseTime(timer.get("NextElapseUSecRealtime"))
	j.State = state(timer, svc)

	if svc == nil || svc.get("LoadState") == "not-found" {
		j.Err = fmt.Sprintf("starts %s, which doesn't exist", j.Unit)
		return j
	}
	if start := svc.get("ExecStart"); start != "" {
		j.Program = field(start, "path", "argv[]")
		j.Command = field(start, "argv[]", "ignore_errors")
	}
	if pid, _ := strconv.Atoi(svc.get("MainPID")); pid > 0 && j.State == job.StateRunning {
		j.PID = pid
	}
	j.LastExit = lastExit(svc)
	return j
}

// vendor reports whether a unit file came with the distribution rather than
// from an administrator or user.
func vendor(path string) bool {
	return strings.HasPrefix(path, "/usr/lib/systemd/") || strings.HasPrefix(path, "/lib/systemd/")
}

func trigger(timer unit) (desc, expr string) {
	var descs, exprs []string
	for _, v := range timer["TimersCalendar"] {
		cal := field(v, "OnCalendar", "next_elapse")
		descs = append(descs, describeCalendar(cal))
		exprs = append(exprs, cal)
	}
	for _, v := range timer["TimersMonotonic"] {
		inner := strings.TrimSpace(strings.Trim(v, "{}"))
		key, rest, ok := strings.Cut(inner, "=")
		if !ok {
			continue
		}
		span, _, _ := strings.Cut(rest, " ; ")
		descs = append(descs, describeMonotonic(key, span))
		exprs = append(exprs, strings.TrimSuffix(key, "USec")+"Sec="+span)
	}
	if len(descs) == 0 {
		return "on demand", ""
	}
	// An interval reads better first: "hourly · 15 minutes after boot".
	slices.SortStableFunc(descs, func(a, b string) int {
		return boolInt(strings.Contains(a, "after")) - boolInt(strings.Contains(b, "after"))
	})
	return strings.Join(descs, " · "), strings.Join(exprs, ", ")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func state(timer, svc unit) job.State {
	switch timer.get("ActiveState") {
	case "active":
		if s := svc.get("ActiveState"); s == "active" || s == "activating" || s == "reloading" {
			return job.StateRunning
		}
		return job.StateIdle
	}
	switch timer.get("UnitFileState") {
	case "disabled", "masked", "masked-runtime":
		return job.StateDisabled
	}
	return job.StateUnloaded
}

// lastExit reads how the service last ended. ExecMainCode follows waitid(2):
// 0 means it has not run, 1 an exit status, 2 and 3 a signal.
func lastExit(svc unit) *int {
	code, _ := strconv.Atoi(svc.get("ExecMainCode"))
	status, err := strconv.Atoi(svc.get("ExecMainStatus"))
	if code == 0 || err != nil {
		return nil
	}
	if code == 2 || code == 3 {
		status = -status
	}
	return &status
}

// parseTime reads timestamps like "Mon 2026-11-09 03:42:00 UTC", which
// systemctl prints in the machine's local zone.
func parseTime(s string) time.Time {
	if s == "" || s == "n/a" {
		return time.Time{}
	}
	t, err := time.ParseInLocation("Mon 2006-01-02 15:04:05 MST", s, time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}
