// Package crontab finds jobs defined in crontab files.
package crontab

import (
	"bufio"
	"io"
	"maps"
	"strings"
)

// Entry is one job line in a crontab.
type Entry struct {
	Line    int
	Expr    string
	User    string
	Command string

	// Err explains why cron can't read the line. The line is kept so it can
	// be reported rather than silently dropped.
	Err string

	// Env holds the variables set above this line, such as MAILTO.
	Env map[string]string
}

// Parse reads crontab lines. System crontabs (/etc/crontab, /etc/cron.d)
// have a user column between the schedule and the command.
func Parse(r io.Reader, system bool) ([]Entry, error) {
	var (
		entries []Entry
		env     = map[string]string{}
		sc      = bufio.NewScanner(r)
		n       int
	)
	for sc.Scan() {
		n++
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' {
			continue
		}
		if k, v, ok := parseEnv(line); ok {
			env = maps.Clone(env)
			env[k] = v
			continue
		}

		e := parseJob(line, system)
		e.Line = n
		e.Env = env
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

func parseJob(line string, system bool) Entry {
	nfields := 5
	if line[0] == '@' {
		nfields = 1
	}
	if system {
		nfields++
	}

	fields, rest := splitFields(line, nfields)
	if len(fields) < nfields || rest == "" {
		need := "a schedule and a command"
		if system {
			need = "a schedule, a user and a command"
		}
		return Entry{Command: line, Err: "it needs " + need}
	}

	e := Entry{Command: rest}
	if system {
		e.User = fields[len(fields)-1]
		fields = fields[:len(fields)-1]
	}
	e.Expr = strings.Join(fields, " ")
	return e
}

// splitFields returns the first n whitespace-separated fields and the rest of
// the line, which keeps the command's own spacing.
func splitFields(s string, n int) ([]string, string) {
	fields := make([]string, 0, n)
	for len(fields) < n {
		s = strings.TrimLeft(s, " \t")
		i := strings.IndexAny(s, " \t")
		if i < 0 {
			if s != "" {
				fields = append(fields, s)
			}
			return fields, ""
		}
		fields = append(fields, s[:i])
		s = s[i:]
	}
	return fields, strings.TrimSpace(s)
}

// parseEnv matches NAME=value lines. Values may be quoted.
func parseEnv(line string) (string, string, bool) {
	k, v, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	k = strings.TrimSpace(k)
	if k == "" || strings.ContainsAny(k, " \t*") {
		return "", "", false
	}
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') && v[len(v)-1] == v[0] {
		v = v[1 : len(v)-1]
	}
	return k, v, true
}
