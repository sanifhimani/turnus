package hermes

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

var now = time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC)

func TestRead(t *testing.T) {
	jobs := read("testdata/jobs.json", now)
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3", len(jobs))
	}

	brief := jobs[0]
	if brief.Name != "morning-brief" || brief.Schedule != "weekdays at 08:00" || brief.Command != "Summarize my unread email" {
		t.Errorf("brief: %+v", brief)
	}
	if !brief.Next.Equal(time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)) || brief.LastResult != "ok" || brief.Host != Host {
		t.Errorf("brief next=%v result=%q host=%q", brief.Next, brief.LastResult, brief.Host)
	}

	watch := jobs[1]
	if watch.Schedule != "every 30 minutes" || watch.LastResult != "error" || watch.LastRun.IsZero() {
		t.Errorf("watch: schedule=%q result=%q last=%v", watch.Schedule, watch.LastResult, watch.LastRun)
	}

	digest := jobs[2]
	if digest.State != job.StateDisabled || !digest.Next.IsZero() || digest.Schedule != "Fridays at 17:00" {
		t.Errorf("digest: state=%q next=%v schedule=%q", digest.State, digest.Next, digest.Schedule)
	}
}

func TestReadMissingAndBroken(t *testing.T) {
	if jobs := read("testdata/missing.json", now); jobs != nil {
		t.Errorf("a missing store should give no jobs, got %v", jobs)
	}
	bad := filepath.Join(t.TempDir(), "jobs.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	jobs := read(bad, now)
	if len(jobs) != 1 || jobs[0].Err == "" {
		t.Errorf("a broken store should be reported, got %+v", jobs)
	}
}

func TestLatestOutput(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"2026-11-08T08-00-00.md", "2026-11-09T08-00-00.md"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got := latestOutput(dir); filepath.Base(got) != "2026-11-09T08-00-00.md" {
		t.Errorf("got %q", got)
	}
}
