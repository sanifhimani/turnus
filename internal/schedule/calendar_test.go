package schedule

import "testing"

func TestCronFromCalendar(t *testing.T) {
	tests := []struct {
		cal  string
		want string
		ok   bool
	}{
		{"*-*-* 02:30:00", "30 02 * * *", true},
		{"*-*-* *:00/5:00", "00/5 * * * *", true},
		{"Mon..Fri *-*-* 09:00:00", "00 09 * * Mon-Fri", true},
		{"Mon,Wed *-*-* 06,18:00:00", "00 06,18 * * Mon,Wed", true},
		{"*-*-01 00:00:00", "00 00 01 * *", true},
		{"*-02-30 00:00:00", "00 00 30 02 *", true},
		{"2027-*-* 00:00:00", "", false},
		{"*-*-* 00:00:30", "", false},
		{"*-*~01 00:00:00", "", false},
		{"daily", "", false},
	}
	for _, tt := range tests {
		got, ok := CronFromCalendar(tt.cal)
		if got != tt.want || ok != tt.ok {
			t.Errorf("CronFromCalendar(%q) = %q, %v; want %q, %v", tt.cal, got, ok, tt.want, tt.ok)
		}
		if ok {
			if _, err := ParseCron(got); err != nil {
				t.Errorf("CronFromCalendar(%q) produced unparsable %q: %v", tt.cal, got, err)
			}
		}
	}
}
