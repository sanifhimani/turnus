// Package source holds helpers shared by the job sources.
package source

import (
	"strconv"
	"strings"
	"time"
)

// Time decodes the timestamp formats agent schedulers write: RFC 3339
// strings, or Unix times in seconds or milliseconds. Anything else, including
// null, decodes to the zero time.
type Time struct{ time.Time }

func (t *Time) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		if n > 1e12 {
			t.Time = time.UnixMilli(int64(n))
		} else {
			t.Time = time.Unix(int64(n), 0)
		}
		return nil
	}
	if p, err := time.Parse(time.RFC3339Nano, s); err == nil {
		t.Time = p
	}
	return nil
}

// Summary shortens a prompt to its first line, for use as a job's command.
func Summary(prompt string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(prompt), "\n")
	if r := []rune(line); len(r) > 200 {
		line = string(r[:199]) + "…"
	}
	return line
}
