package source

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTime(t *testing.T) {
	want := time.Date(2026, 11, 9, 14, 30, 0, 0, time.UTC)
	for _, in := range []string{`"2026-11-09T14:30:00Z"`, `1794234600`, `1794234600000`} {
		var got Time
		if err := json.Unmarshal([]byte(in), &got); err != nil || !got.Equal(want) {
			t.Errorf("%s: got %v, %v", in, got.Time, err)
		}
	}
	for _, in := range []string{`null`, `""`, `"soon"`} {
		var got Time
		if err := json.Unmarshal([]byte(in), &got); err != nil || !got.IsZero() {
			t.Errorf("%s: got %v, %v; want zero", in, got.Time, err)
		}
	}
}

func TestSummary(t *testing.T) {
	if got := Summary("  Check the deploy\nThen post to Slack"); got != "Check the deploy" {
		t.Errorf("got %q", got)
	}
}
