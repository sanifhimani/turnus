package schedule

import (
	"strconv"
	"strings"
)

var rruleDays = map[string]string{"SU": "0", "MO": "1", "TU": "2", "WE": "3", "TH": "4", "FR": "5", "SA": "6"}

// CronFromRRule converts an iCalendar recurrence rule (RFC 5545) to a cron
// expression. It handles minutely, hourly, daily, weekly and monthly rules
// built from BYMINUTE, BYHOUR, BYDAY and BYMONTHDAY, and reports false for
// rules cron can't express, such as every second week.
//
//	FREQ=WEEKLY;BYDAY=TU,TH;BYHOUR=17;BYMINUTE=0  ->  0 17 * * 2,4
func CronFromRRule(rule string) (string, bool) {
	parts := map[string]string{}
	for kv := range strings.SplitSeq(strings.TrimPrefix(strings.TrimSpace(rule), "RRULE:"), ";") {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return "", false
		}
		parts[strings.ToUpper(k)] = v
	}

	interval := 1
	if v, ok := parts["INTERVAL"]; ok {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return "", false
		}
		interval = n
	}
	for k, v := range parts {
		switch k {
		case "FREQ", "INTERVAL", "BYMINUTE", "BYHOUR", "BYDAY", "BYMONTHDAY", "WKST":
		case "BYSECOND":
			if v != "0" {
				return "", false
			}
		default:
			// COUNT, UNTIL, BYSETPOS and the rest change which runs happen.
			return "", false
		}
	}

	minute, hour, dom, dow := or(parts["BYMINUTE"], "0"), or(parts["BYHOUR"], "0"), "*", "*"
	if days, ok := parts["BYDAY"]; ok {
		var list []string
		for d := range strings.SplitSeq(days, ",") {
			n, ok := rruleDays[strings.ToUpper(d)]
			if !ok {
				return "", false // "1MO" and other numbered days
			}
			list = append(list, n)
		}
		dow = strings.Join(list, ",")
	}

	every := func(n int) string {
		if n == 1 {
			return "*"
		}
		return "*/" + strconv.Itoa(n)
	}
	switch parts["FREQ"] {
	case "MINUTELY":
		minute, hour = every(interval), or(parts["BYHOUR"], "*")
	case "HOURLY":
		hour = or(parts["BYHOUR"], every(interval))
	case "DAILY", "WEEKLY", "MONTHLY":
		if interval != 1 {
			return "", false
		}
		switch parts["FREQ"] {
		case "WEEKLY":
			if dow == "*" {
				return "", false // the day comes from DTSTART, which isn't here
			}
		case "MONTHLY":
			if dom = parts["BYMONTHDAY"]; dom == "" {
				return "", false
			}
		}
	default:
		return "", false
	}

	expr := strings.Join([]string{minute, hour, dom, "*", dow}, " ")
	if _, err := ParseCron(expr); err != nil {
		return "", false
	}
	return expr, true
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
