package systemd

import (
	"bufio"
	"io"
	"strings"
)

// unit holds the properties `systemctl show` printed for one unit. Some
// properties, like TimersMonotonic, can appear more than once.
type unit map[string][]string

func (u unit) get(key string) string {
	if v := u[key]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// parseShow reads `systemctl show` output: KEY=value lines, with a blank
// line between units.
func parseShow(r io.Reader) map[string]unit {
	units := map[string]unit{}
	cur := unit{}
	flush := func() {
		if id := cur.get("Id"); id != "" {
			units[id] = cur
		}
		cur = unit{}
	}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			flush()
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			cur[k] = append(cur[k], v)
		}
	}
	flush()
	return units
}

// field returns one field from a structured value such as
// "{ path=/bin/sh ; argv[]=/bin/sh -c x ; ignore_errors=no }". The value
// ends where the next known field starts, because commands can contain
// " ; " themselves.
func field(v, name, next string) string {
	_, rest, ok := strings.Cut(v, name+"=")
	if !ok {
		return ""
	}
	if next != "" {
		if val, _, ok := strings.Cut(rest, " ; "+next+"="); ok {
			return val
		}
	}
	val, _, _ := strings.Cut(rest, " ; ")
	return strings.TrimSuffix(strings.TrimSpace(val), " }")
}
