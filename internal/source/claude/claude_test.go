package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

var now = time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)

func TestRead(t *testing.T) {
	jobs := read("testdata/scheduled-tasks.json", now)
	if len(jobs) != 4 {
		t.Fatalf("got %d jobs, want 4", len(jobs))
	}
	byID := map[string]job.Job{}
	for _, j := range jobs {
		byID[j.ID] = j
	}

	triage := byID["claude:triage-inbox"]
	if triage.Name != "Triage inbox" || triage.Schedule != "weekdays at 09:00" || triage.Host != Host {
		t.Errorf("triage: %+v", triage)
	}
	if triage.Command != "Sort new email into folders and flag anything urgent" || triage.File != "testdata/triage-inbox/SKILL.md" {
		t.Errorf("triage command=%q file=%q", triage.Command, triage.File)
	}
	if triage.LastResult != "" {
		t.Errorf("a skip older than the last run should be ignored, got %q", triage.LastResult)
	}

	notes := byID["claude:release-notes"]
	if notes.LastResult != "folder_missing" {
		t.Errorf("release notes should report its newest skip, got %q", notes.LastResult)
	}

	oneOff := byID["claude:one-off"]
	if oneOff.Next.IsZero() || oneOff.Schedule[:8] != "once at " {
		t.Errorf("one-off: schedule=%q next=%v", oneOff.Schedule, oneOff.Next)
	}

	manual := byID["claude:by-hand"]
	if manual.Schedule != "manual" || manual.State != job.StateDisabled || !manual.Next.IsZero() {
		t.Errorf("manual: %+v", manual)
	}
}

// empty-from-app.json is a real file written by Claude Desktop 4.23118.0,
// copied as is. If an update changes the format, this is where it shows.
func TestReadFileFromApp(t *testing.T) {
	jobs := read("testdata/empty-from-app.json", now)
	if len(jobs) != 0 {
		t.Errorf("a store with no tasks should give no jobs, got %+v", jobs)
	}
}

func TestReadEmptyAndBroken(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.json")
	broken := filepath.Join(dir, "broken.json")
	os.WriteFile(empty, []byte(`{"scheduledTasks":[],"recordedSkips":{}}`), 0o600)
	os.WriteFile(broken, []byte(`{"scheduledTasks": "surprise"}`), 0o600)

	if jobs := read(empty, now); len(jobs) != 0 {
		t.Errorf("an empty store should give no jobs, got %v", jobs)
	}
	if jobs := read(broken, now); len(jobs) != 1 || jobs[0].Err == "" {
		t.Errorf("a store in an unknown format should be reported, got %+v", jobs)
	}
	if jobs := read(filepath.Join(dir, "missing.json"), now); jobs != nil {
		t.Errorf("a missing store should give no jobs, got %v", jobs)
	}
}

func TestDescribePrompt(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "SKILL.md")
	os.WriteFile(plain, []byte("\n\nCheck the deploy dashboard\nand report\n"), 0o600)
	if got := describePrompt(plain); got != "Check the deploy dashboard" {
		t.Errorf("got %q", got)
	}
	if got := describePrompt(filepath.Join(dir, "none.md")); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestReadDates(t *testing.T) {
	jobs := read("testdata/scheduled-tasks.json", now)
	byID := map[string]job.Job{}
	for _, j := range jobs {
		byID[strings.TrimPrefix(j.ID, "claude:")] = j
	}
	created := time.UnixMilli(1793000000000)
	for _, id := range []string{"triage-inbox", "release-notes", "one-off", "by-hand"} {
		if !byID[id].Created.Equal(created) {
			t.Errorf("%s: created = %v, want %v", id, byID[id].Created, created)
		}
	}
	if once := byID["one-off"].Once; !once.Equal(time.UnixMilli(1794330000000)) {
		t.Errorf("one-off: once = %v", once)
	}
	if !byID["triage-inbox"].Once.IsZero() {
		t.Error("a recurring task has no one-time run")
	}
}
