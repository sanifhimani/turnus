package systemd

import (
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/sanifhimani/turnus/internal/schedule"
)

// describeCalendar returns plain English for an OnCalendar expression, or
// the expression itself when there is no short description.
func describeCalendar(cal string) string {
	if expr, ok := schedule.CronFromCalendar(cal); ok {
		if c, err := schedule.ParseCron(expr); err == nil {
			if d := c.Describe(); d != "" {
				return d
			}
		}
	}
	return cal
}

// describeMonotonic phrases timers that count from an event rather than the
// clock, such as OnUnitActiveSec=1h.
func describeMonotonic(key, span string) string {
	d, ok := parseTimespan(span)
	if !ok {
		return key + "=" + span
	}
	switch key {
	case "OnUnitActiveUSec", "OnUnitInactiveUSec":
		return schedule.Every(d)
	case "OnBootUSec":
		return humanSpan(d) + " after boot"
	case "OnStartupUSec":
		return humanSpan(d) + " after startup"
	case "OnActiveUSec":
		return humanSpan(d) + " after the timer starts"
	}
	return key + "=" + span
}

// humanSpan writes a duration in its largest whole unit: "15 minutes",
// "1 hour", "90 seconds".
func humanSpan(d time.Duration) string {
	for _, u := range []struct {
		d    time.Duration
		name string
	}{{24 * time.Hour, "day"}, {time.Hour, "hour"}, {time.Minute, "minute"}} {
		if d >= u.d && d%u.d == 0 {
			return count(int(d/u.d), u.name)
		}
	}
	return count(int(d/time.Second), "second")
}

func count(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

var spanUnits = map[string]time.Duration{
	"us": time.Microsecond, "ms": time.Millisecond, "s": time.Second,
	"min": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour,
	"M": 30*24*time.Hour + 10*time.Hour + 30*time.Minute, // systemd's month: 30.44 days
	"y": 365*24*time.Hour + 6*time.Hour,
}

// parseTimespan reads systemd time spans such as "15min", "1h 30min", or
// "1d 2h 3.5s".
func parseTimespan(s string) (time.Duration, bool) {
	var total time.Duration
	parts := strings.Fields(s)
	if len(parts) == 0 {
		return 0, false
	}
	for _, p := range parts {
		i := strings.IndexFunc(p, func(r rune) bool { return unicode.IsLetter(r) })
		if i <= 0 {
			return 0, false
		}
		n, err := strconv.ParseFloat(p[:i], 64)
		unit, ok := spanUnits[p[i:]]
		if err != nil || !ok {
			return 0, false
		}
		total += time.Duration(n * float64(unit))
	}
	return total, true
}
