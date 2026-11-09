package schedule

import "testing"

func TestDescribe(t *testing.T) {
	tests := []struct{ expr, want string }{
		{"* * * * *", "every minute"},
		{"*/5 * * * *", "every 5 minutes"},
		{"0 * * * *", "hourly"},
		{"15 * * * *", "hourly at :15"},
		{"0 */6 * * *", "every 6 hours"},
		{"0 9 * * *", "daily at 09:00"},
		{"@daily", "daily at 00:00"},
		{"30 8,17 * * *", "daily at 08:30 and 17:30"},
		{"0 9 * * 1-5", "weekdays at 09:00"},
		{"0 10 * * 0,6", "weekends at 10:00"},
		{"0 9 * * mon", "Mondays at 09:00"},
		{"0 9 * * 1,3,5", "Mon, Wed and Fri at 09:00"},
		{"*/10 * * * 1-5", "every 10 minutes on weekdays"},
		{"0 3 1 * *", "monthly on the 1st at 03:00"},
		{"0 3 22 * *", "monthly on the 22nd at 03:00"},
		{"0 0 1 1 *", "yearly on Jan 1 at 00:00"},
		{"@reboot", "at boot"},
		{"7,19,41 * * * *", ""},
		{"0 9 1-7 * 1", ""},
	}
	for _, tt := range tests {
		c, err := ParseCron(tt.expr)
		if err != nil {
			t.Fatalf("ParseCron(%q): %v", tt.expr, err)
		}
		if got := c.Describe(); got != tt.want {
			t.Errorf("Describe(%q) = %q, want %q", tt.expr, got, tt.want)
		}
	}
}
