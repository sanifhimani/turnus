// Package schedule parses scheduler expressions, computes upcoming run
// times, and describes schedules in plain English.
package schedule

import (
	"errors"
	"fmt"
	"math/bits"
	"strconv"
	"strings"
	"time"
)

// field is a set of allowed values for one cron field, stored as a bitmask.
type field struct {
	bits     uint64
	min, max int
}

func (f field) has(v int) bool { return f.bits&(1<<uint(v)) != 0 }

func (f field) all() bool { return f.bits == span(f.min, f.max, 1) }

func (f field) count() int { return bits.OnesCount64(f.bits) }

// values returns the set members in ascending order.
func (f field) values() []int {
	out := make([]int, 0, f.count())
	for v := f.min; v <= f.max; v++ {
		if f.has(v) {
			out = append(out, v)
		}
	}
	return out
}

// step reports whether the field is exactly min, min+n, min+2n, ... for some
// n > 1, as written by "*/n".
func (f field) step() (int, bool) {
	vs := f.values()
	if len(vs) < 2 || vs[0] != f.min {
		return 0, false
	}
	n := vs[1] - vs[0]
	if n < 2 || f.bits != span(f.min, f.max, n) {
		return 0, false
	}
	return n, true
}

// contiguous reports whether the field is a single unbroken range, such as
// 9-17.
func (f field) contiguous() bool {
	vs := f.values()
	return len(vs) > 1 && vs[len(vs)-1]-vs[0] == len(vs)-1
}

func span(lo, hi, step int) uint64 {
	var b uint64
	for v := lo; v <= hi; v += step {
		b |= 1 << uint(v)
	}
	return b
}

// Cron is a parsed five-field cron expression.
type Cron struct {
	minute, hour, dom, month, dow field

	// Reboot is set for "@reboot", which has no recurring schedule.
	Reboot bool

	// When both day fields are restricted, cron runs on days matching
	// either one. Otherwise both must match.
	domStar, dowStar bool
}

var macros = map[string]string{
	"@yearly":   "0 0 1 1 *",
	"@annually": "0 0 1 1 *",
	"@monthly":  "0 0 1 * *",
	"@weekly":   "0 0 * * 0",
	"@daily":    "0 0 * * *",
	"@midnight": "0 0 * * *",
	"@hourly":   "0 * * * *",
}

var (
	monthNames = []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"}
	dayNames   = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
)

// ParseCron parses a standard five-field expression or one of the @ macros.
func ParseCron(expr string) (*Cron, error) {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "@") {
		if expr == "@reboot" {
			return &Cron{Reboot: true}, nil
		}
		m, ok := macros[expr]
		if !ok {
			return nil, fmt.Errorf("unknown macro %q", expr)
		}
		expr = m
	}

	parts := strings.Fields(expr)
	if len(parts) != 5 {
		return nil, fmt.Errorf("expected 5 fields, got %d", len(parts))
	}

	c := &Cron{}
	var err error
	if c.minute, err = parseField(parts[0], 0, 59, nil); err != nil {
		return nil, fmt.Errorf("minute: %w", err)
	}
	if c.hour, err = parseField(parts[1], 0, 23, nil); err != nil {
		return nil, fmt.Errorf("hour: %w", err)
	}
	if c.dom, err = parseField(parts[2], 1, 31, nil); err != nil {
		return nil, fmt.Errorf("day of month: %w", err)
	}
	if c.month, err = parseField(parts[3], 1, 12, monthNames); err != nil {
		return nil, fmt.Errorf("month: %w", err)
	}
	// Day of week accepts 7 as Sunday; fold it onto 0 after parsing.
	if c.dow, err = parseField(parts[4], 0, 7, dayNames); err != nil {
		return nil, fmt.Errorf("day of week: %w", err)
	}
	if c.dow.has(7) {
		c.dow.bits = c.dow.bits&^(1<<7) | 1
	}
	c.dow.max = 6

	c.domStar = parts[2] == "*" || parts[2] == "?"
	c.dowStar = parts[4] == "*" || parts[4] == "?"
	return c, nil
}

func parseField(s string, lo, hi int, names []string) (field, error) {
	f := field{min: lo, max: hi}
	for item := range strings.SplitSeq(s, ",") {
		b, err := parseItem(item, lo, hi, names)
		if err != nil {
			return f, err
		}
		f.bits |= b
	}
	return f, nil
}

func parseItem(item string, lo, hi int, names []string) (uint64, error) {
	rng, stepStr, hasStep := strings.Cut(item, "/")
	step := 1
	if hasStep {
		n, err := strconv.Atoi(stepStr)
		if err != nil || n < 1 {
			return 0, fmt.Errorf("bad step %q", stepStr)
		}
		step = n
	}

	var start, end int
	switch {
	case rng == "*" || rng == "?":
		start, end = lo, hi
	case strings.Contains(rng, "-"):
		a, b, _ := strings.Cut(rng, "-")
		var err error
		if start, err = parseValue(a, lo, hi, names); err != nil {
			return 0, err
		}
		if end, err = parseValue(b, lo, hi, names); err != nil {
			return 0, err
		}
		if start > end {
			return 0, fmt.Errorf("range %q is backwards", rng)
		}
	default:
		v, err := parseValue(rng, lo, hi, names)
		if err != nil {
			return 0, err
		}
		start, end = v, v
		// "5/15" means from 5 to the end, every 15.
		if hasStep {
			end = hi
		}
	}
	return span(start, end, step), nil
}

func parseValue(s string, lo, hi int, names []string) (int, error) {
	if s == "" {
		return 0, errors.New("empty value")
	}
	for i, n := range names {
		if strings.EqualFold(s, n) {
			// Month names start at 1, day names at 0.
			return i + lo, nil
		}
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("bad value %q", s)
	}
	if v < lo || v > hi {
		return 0, fmt.Errorf("%d is out of range %d-%d", v, lo, hi)
	}
	return v, nil
}

// maxSearch bounds Next for expressions that rarely or never match, such as
// February 30th.
const maxSearch = 5 * 366 * 24 * time.Hour

// Next returns the first run strictly after t, in t's location. It returns
// the zero time for @reboot or expressions that never match.
func (c *Cron) Next(t time.Time) time.Time {
	if c.Reboot {
		return time.Time{}
	}
	loc := t.Location()
	limit := t.Add(maxSearch)
	t = t.Truncate(time.Minute).Add(time.Minute)

	for t.Before(limit) {
		if !c.month.has(int(t.Month())) {
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
			continue
		}
		if !c.dayMatches(t) {
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, loc)
			continue
		}
		if !c.hour.has(t.Hour()) {
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, loc)
			continue
		}
		if !c.minute.has(t.Minute()) {
			t = t.Add(time.Minute)
			continue
		}
		return t
	}
	return time.Time{}
}

func (c *Cron) dayMatches(t time.Time) bool {
	dom := c.dom.has(t.Day())
	dow := c.dow.has(int(t.Weekday()))
	if !c.domStar && !c.dowStar {
		return dom || dow
	}
	return dom && dow
}
