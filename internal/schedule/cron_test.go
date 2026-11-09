package schedule

import (
	"slices"
	"testing"
	"time"
)

func TestParseCronErrors(t *testing.T) {
	for _, expr := range []string{
		"",
		"* * * *",
		"* * * * * *",
		"60 * * * *",
		"* 24 * * *",
		"* * 0 * *",
		"* * * 13 *",
		"* * * * 8",
		"*/0 * * * *",
		"5-1 * * * *",
		"a * * * *",
		"@sometimes",
	} {
		if _, err := ParseCron(expr); err == nil {
			t.Errorf("ParseCron(%q) succeeded, want error", expr)
		}
	}
}

func TestNext(t *testing.T) {
	loc := time.UTC
	from := time.Date(2026, 11, 8, 21, 30, 15, 0, loc) // a Sunday

	tests := []struct {
		expr string
		want time.Time
	}{
		{"* * * * *", time.Date(2026, 11, 8, 21, 31, 0, 0, loc)},
		{"*/15 * * * *", time.Date(2026, 11, 8, 21, 45, 0, 0, loc)},
		{"0 * * * *", time.Date(2026, 11, 8, 22, 0, 0, 0, loc)},
		{"0 9 * * *", time.Date(2026, 11, 9, 9, 0, 0, 0, loc)},
		{"0 9 * * 1-5", time.Date(2026, 11, 9, 9, 0, 0, 0, loc)},
		{"0 9 * * sat", time.Date(2026, 11, 14, 9, 0, 0, 0, loc)},
		{"0 0 * * 7", time.Date(2026, 11, 15, 0, 0, 0, 0, loc)},
		{"30 4 1 * *", time.Date(2026, 12, 1, 4, 30, 0, 0, loc)},
		{"0 0 1 jan *", time.Date(2027, 1, 1, 0, 0, 0, 0, loc)},
		{"@daily", time.Date(2026, 11, 9, 0, 0, 0, 0, loc)},
		{"5/20 * * * *", time.Date(2026, 11, 8, 21, 45, 0, 0, loc)},
		// Both day fields restricted: either may match. The 10th comes
		// before the next Friday (the 13th).
		{"0 0 10 * fri", time.Date(2026, 11, 10, 0, 0, 0, 0, loc)},
		{"0 0 29 2 *", time.Date(2028, 2, 29, 0, 0, 0, 0, loc)},
	}
	for _, tt := range tests {
		c, err := ParseCron(tt.expr)
		if err != nil {
			t.Fatalf("ParseCron(%q): %v", tt.expr, err)
		}
		if got := c.Next(from); !got.Equal(tt.want) {
			t.Errorf("Next(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func TestNextNeverMatches(t *testing.T) {
	c, err := ParseCron("0 0 30 2 *")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Next(time.Now()); !got.IsZero() {
		t.Errorf("Next = %v, want zero", got)
	}
}

func TestNextReboot(t *testing.T) {
	c, err := ParseCron("@reboot")
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Next(time.Now()); !got.IsZero() {
		t.Errorf("Next = %v, want zero", got)
	}
}

func TestHours(t *testing.T) {
	tests := []struct {
		expr string
		want []int
	}{
		{"* * * * *", nil},
		{"30 2 * * *", []int{2}},
		{"0 1-3 * * *", []int{1, 2, 3}},
		{"@reboot", nil},
	}
	for _, tt := range tests {
		c, err := ParseCron(tt.expr)
		if err != nil {
			t.Fatal(err)
		}
		if got := c.Hours(); !slices.Equal(got, tt.want) {
			t.Errorf("Hours(%q) = %v, want %v", tt.expr, got, tt.want)
		}
	}
}

func BenchmarkNext(b *testing.B) {
	c, _ := ParseCron("0 9 * * 1-5")
	from := time.Date(2026, 11, 8, 21, 30, 0, 0, time.UTC)
	for b.Loop() {
		c.Next(from)
	}
}
