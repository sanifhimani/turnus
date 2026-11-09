package schedule

import (
	"testing"
	"time"
)

func TestEvery(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, ""},
		{30 * time.Second, "every 30 seconds"},
		{time.Minute, "every minute"},
		{90 * time.Second, "every 90 seconds"},
		{15 * time.Minute, "every 15 minutes"},
		{time.Hour, "hourly"},
		{6 * time.Hour, "every 6 hours"},
		{24 * time.Hour, "daily"},
		{3 * 24 * time.Hour, "every 3 days"},
		{7 * 24 * time.Hour, "weekly"},
	}
	for _, tt := range tests {
		if got := Every(tt.d); got != tt.want {
			t.Errorf("Every(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}
