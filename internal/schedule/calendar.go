package schedule

import (
	"strings"
	"unicode"
)

// CronFromCalendar converts a normalized OnCalendar expression, as systemctl
// prints it, to a five-field cron expression. It handles the common shape
// "[weekdays] *-month-day hour:minute:00" and reports false for anything
// cron can't express, such as specific years, seconds, or "~" for days
// counted from the end of the month.
//
//	Mon..Fri *-*-* 09:00:00  ->  0 9 * * Mon-Fri
//	*-*-* *:00/5:00          ->  00/5 * * * *
func CronFromCalendar(cal string) (string, bool) {
	f := strings.Fields(cal)
	dow := "*"
	if len(f) == 3 && unicode.IsLetter(rune(f[0][0])) {
		dow, f = f[0], f[1:]
	}
	if len(f) != 2 || strings.Contains(cal, "~") {
		return "", false
	}

	date := strings.Split(f[0], "-")
	clock := strings.Split(f[1], ":")
	if len(date) != 3 || len(clock) != 3 || date[0] != "*" {
		return "", false
	}
	if sec := clock[2]; sec != "00" && sec != "0" {
		return "", false
	}

	fields := []string{clock[1], clock[0], date[2], date[1], dow}
	for i, v := range fields {
		fields[i] = strings.ReplaceAll(v, "..", "-")
	}
	return strings.Join(fields, " "), true
}
