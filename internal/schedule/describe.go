package schedule

import (
	"fmt"
	"strings"
)

var (
	dayTitles   = []string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
	monthTitles = []string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
)

// Describe returns the schedule in plain English, such as "weekdays at
// 09:00" or "every 15 minutes". It returns "" if the schedule has no short
// description.
func (c *Cron) Describe() string {
	if c.Reboot {
		return "at boot"
	}
	clock := c.describeClock()
	if clock == "" {
		return ""
	}
	days, ok := c.describeDays()
	if !ok {
		return ""
	}

	switch {
	case days == "":
		if strings.HasPrefix(clock, "at ") {
			return "daily " + clock
		}
		return clock
	case strings.HasPrefix(clock, "at "):
		return days + " " + clock
	default:
		return clock + " " + onDays(days)
	}
}

// describeClock covers the minute and hour fields.
func (c *Cron) describeClock() string {
	m, h := c.minute, c.hour
	switch {
	case m.all() && h.all():
		return "every minute"
	case h.all():
		if n, ok := m.step(); ok {
			return fmt.Sprintf("every %d minutes", n)
		}
		if m.count() == 1 {
			v := m.values()[0]
			if v == 0 {
				return "hourly"
			}
			return fmt.Sprintf("hourly at :%02d", v)
		}
	case m.count() == 1 && m.values()[0] == 0:
		if n, ok := h.step(); ok {
			return fmt.Sprintf("every %d hours", n)
		}
	}

	if m.count() == 1 && h.count() <= 4 {
		minute := m.values()[0]
		times := make([]string, 0, h.count())
		for _, hr := range h.values() {
			times = append(times, fmt.Sprintf("%02d:%02d", hr, minute))
		}
		return "at " + join(times)
	}
	return ""
}

// describeDays covers day of month, month, and day of week. An empty string
// with ok=true means every day.
func (c *Cron) describeDays() (string, bool) {
	domAll, monAll, dowAll := c.dom.all(), c.month.all(), c.dow.all()

	switch {
	case domAll && monAll && dowAll:
		return "", true
	case domAll && monAll:
		return describeWeekdays(c.dow), true
	case dowAll && c.dom.count() == 1 && monAll:
		return "monthly on the " + ordinal(c.dom.values()[0]), true
	case dowAll && c.dom.count() == 1 && c.month.count() == 1:
		return fmt.Sprintf("yearly on %s %d", monthTitles[c.month.values()[0]], c.dom.values()[0]), true
	}
	return "", false
}

func describeWeekdays(f field) string {
	switch f.bits {
	case span(1, 5, 1):
		return "weekdays"
	case 1 | 1<<6:
		return "weekends"
	}
	vs := f.values()
	if len(vs) == 1 {
		return dayTitles[vs[0]] + "s"
	}
	names := make([]string, len(vs))
	for i, v := range vs {
		names[i] = dayTitles[v][:3]
	}
	return join(names)
}

// onDays adapts a day phrase to follow a frequency, as in "every 5 minutes
// on weekdays".
func onDays(days string) string {
	if rest, ok := strings.CutPrefix(days, "monthly on "); ok {
		return "on " + rest + " of the month"
	}
	if rest, ok := strings.CutPrefix(days, "yearly on "); ok {
		return "on " + rest
	}
	return "on " + days
}

func join(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

func ordinal(n int) string {
	suffix := "th"
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		suffix = "st"
	case n%10 == 2:
		suffix = "nd"
	case n%10 == 3:
		suffix = "rd"
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
