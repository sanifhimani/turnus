package schedule

import (
	"fmt"
	"time"
)

// Every describes a fixed interval, such as "every 30 minutes" or "hourly".
func Every(d time.Duration) string {
	switch {
	case d <= 0:
		return ""
	case d == time.Hour:
		return "hourly"
	case d == 24*time.Hour:
		return "daily"
	case d == 7*24*time.Hour:
		return "weekly"
	case d%(24*time.Hour) == 0:
		return plural(int(d/(24*time.Hour)), "day")
	case d%time.Hour == 0:
		return plural(int(d/time.Hour), "hour")
	case d%time.Minute == 0:
		return plural(int(d/time.Minute), "minute")
	}
	return plural(int(d/time.Second), "second")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "every " + unit
	}
	return fmt.Sprintf("every %d %ss", n, unit)
}
