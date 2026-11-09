package schedule

import "testing"

func TestCronFromRRule(t *testing.T) {
	tests := []struct {
		rule string
		want string
		ok   bool
	}{
		{"FREQ=WEEKLY;BYDAY=TU;BYHOUR=17;BYMINUTE=0", "0 17 * * 2", true},
		{"RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR;BYHOUR=9;BYMINUTE=30", "30 9 * * 1,3,5", true},
		{"FREQ=DAILY;BYHOUR=8;BYMINUTE=0", "0 8 * * *", true},
		{"FREQ=DAILY;BYDAY=MO,TU,WE,TH,FR;BYHOUR=9;BYMINUTE=0", "0 9 * * 1,2,3,4,5", true},
		{"FREQ=HOURLY;INTERVAL=2;BYMINUTE=15", "15 */2 * * *", true},
		{"FREQ=MINUTELY;INTERVAL=30", "*/30 * * * *", true},
		{"FREQ=MONTHLY;BYMONTHDAY=1;BYHOUR=6;BYMINUTE=0", "0 6 1 * *", true},
		{"FREQ=WEEKLY;INTERVAL=2;BYDAY=MO;BYHOUR=9", "", false},
		{"FREQ=WEEKLY;BYHOUR=9", "", false},
		{"FREQ=MONTHLY;BYDAY=1MO", "", false},
		{"FREQ=DAILY;COUNT=5", "", false},
		{"FREQ=YEARLY", "", false},
		{"nonsense", "", false},
	}
	for _, tt := range tests {
		got, ok := CronFromRRule(tt.rule)
		if got != tt.want || ok != tt.ok {
			t.Errorf("CronFromRRule(%q) = %q, %v; want %q, %v", tt.rule, got, ok, tt.want, tt.ok)
		}
	}
}
