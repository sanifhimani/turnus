package codex

import (
	"testing"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

func TestDiscover(t *testing.T) {
	t.Setenv("CODEX_HOME", "testdata")
	now := time.Date(2026, 11, 9, 14, 0, 0, 0, time.Local) // a Monday
	jobs, err := Discover(now)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]job.Job{}
	for _, j := range jobs {
		byName[j.Name] = j
	}
	if len(jobs) != 3 {
		t.Fatalf("got %d jobs, want 3 (the cloud one is left out): %v", len(jobs), byName)
	}

	standup := byName["Standup notes"]
	if standup.Schedule != "weekdays at 09:00" || standup.Host != Host || standup.Command != "Summarize yesterday's commits for standup" {
		t.Errorf("standup: %+v", standup)
	}
	if want := time.Date(2026, 11, 10, 9, 0, 0, 0, time.Local); !standup.Next.Equal(want) {
		t.Errorf("standup next = %v, want %v", standup.Next, want)
	}

	paused := byName["Dependency check"]
	if paused.State != job.StateDisabled || paused.Schedule != "FREQ=WEEKLY;INTERVAL=2;BYDAY=MO;BYHOUR=10" || !paused.Next.IsZero() {
		t.Errorf("paused: %+v", paused)
	}

	if weird := byName["weird"]; weird.Err == "" {
		t.Errorf("an unreadable automation should be reported: %+v", weird)
	}
}
