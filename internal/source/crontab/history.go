package crontab

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
)

// historyWindow bounds how far back turnus reads cron's log. Jobs due less
// often than this show no last run, which is better than reading a month of
// every-minute jobs on every start.
const historyWindow = 35 * 24 * time.Hour

// Log files cron writes to when there's no journal: syslog on Debian and
// Ubuntu, and cron on Fedora and Red Hat.
var historyFiles = []string{"/var/log/cron", "/var/log/cron.log", "/var/log/syslog"}

// history is what cron's log says about past runs. cron logs a line when it
// starts each job, but not how the job ended, so this tells when jobs ran,
// not whether they worked.
type history struct {
	// since is the oldest time the log covers. Runs before it are unknown.
	since time.Time
	runs  []run
}

type run struct {
	user, cmd string
	at        time.Time
}

// cmdLine matches cron's start line in the forms Debian's cron and cronie
// write, with the pid prefix Debian adds when started with -L:
//
//	(root) CMD (/usr/bin/backup)
//	(root) CMD ([3533] /usr/bin/backup)
var cmdLine = regexp.MustCompile(`^\(([^)]+)\) CMD \((?:\[\d+\] )?(.*)\)$`)

// readHistory reads cron's log from the systemd journal, or from the log
// files when the journal has nothing. It returns false when neither can be
// read, as on macOS or for users without access to the system log.
func readHistory(ctx context.Context, now time.Time) (history, bool) {
	if h, ok := journalHistory(ctx, now); ok {
		return h, true
	}
	for _, path := range historyFiles {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		h, ok := parseSyslog(f, now)
		f.Close()
		if ok {
			return h, true
		}
	}
	return history{}, false
}

func journalHistory(ctx context.Context, now time.Time) (history, bool) {
	if _, err := exec.LookPath("journalctl"); err != nil {
		return history{}, false
	}
	since := now.Add(-historyWindow).Format("2006-01-02 15:04:05")
	var out bytes.Buffer
	// Match cron's syslog tags rather than its unit, which each distribution
	// names differently: cron.service on Debian, crond.service on Fedora,
	// cronie.service on Arch.
	args := []string{"--quiet", "--no-pager", "--output=json", "--since=" + since}
	for _, tag := range cronTags {
		args = append(args, "SYSLOG_IDENTIFIER="+tag)
	}
	cmd := exec.CommandContext(ctx, "journalctl", args...)
	cmd.Stdout = &out
	if cmd.Run() != nil {
		return history{}, false
	}
	return parseJournal(&out)
}

// parseJournal reads journalctl's JSON output, one entry per line. Any entry
// from cron's unit, such as its startup line, marks the log as covering
// that time.
func parseJournal(r io.Reader) (history, bool) {
	var h history
	dec := json.NewDecoder(r)
	for {
		var e struct {
			Message   any    `json:"MESSAGE"`
			Timestamp string `json:"__REALTIME_TIMESTAMP"`
		}
		if err := dec.Decode(&e); err != nil {
			break
		}
		usec, err := strconv.ParseInt(e.Timestamp, 10, 64)
		if err != nil {
			continue
		}
		at := time.UnixMicro(usec)
		if h.since.IsZero() || at.Before(h.since) {
			h.since = at
		}
		// journalctl writes messages with unprintable bytes as arrays.
		msg, ok := e.Message.(string)
		if !ok {
			continue
		}
		if m := cmdLine.FindStringSubmatch(msg); m != nil {
			h.runs = append(h.runs, run{user: m[1], cmd: m[2], at: at})
		}
	}
	return h, !h.since.IsZero()
}

// parseSyslog reads a syslog file, keeping cron's lines. rsyslog writes
// timestamps either as RFC 3339 or in the traditional form, which has no
// year.
func parseSyslog(r io.Reader, now time.Time) (history, bool) {
	var h history
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	first := true
	for sc.Scan() {
		line := sc.Text()
		at, rest, ok := syslogTime(line, now)
		if !ok {
			continue
		}
		// The file covers everything from its first line on, whatever
		// program wrote it.
		if first {
			h.since, first = at, false
		}
		// host CRON[3534]: (root) CMD (...)
		_, rest, _ = strings.Cut(rest, " ")
		tag, msg, ok := strings.Cut(rest, ": ")
		if !ok || !isCronTag(tag) {
			continue
		}
		if m := cmdLine.FindStringSubmatch(msg); m != nil {
			h.runs = append(h.runs, run{user: m[1], cmd: m[2], at: at})
		}
	}
	return h, !h.since.IsZero()
}

// cronTags are the syslog tags cron writes under: cron and CRON on Debian,
// crond and CROND for cronie on Fedora and Arch.
var cronTags = []string{"cron", "CRON", "crond", "CROND"}

// isCronTag matches a syslog tag such as CRON[123] or crond[812].
func isCronTag(tag string) bool {
	name, _, _ := strings.Cut(tag, "[")
	return slices.Contains(cronTags, name)
}

func syslogTime(line string, now time.Time) (time.Time, string, bool) {
	stamp, rest, ok := strings.Cut(line, " ")
	if !ok {
		return time.Time{}, "", false
	}
	if t, err := time.Parse(time.RFC3339Nano, stamp); err == nil {
		return t, rest, true
	}
	// Nov 13 17:04:01 host ...; single-digit days are padded with a space.
	if len(line) < 16 {
		return time.Time{}, "", false
	}
	t, err := time.ParseInLocation("Jan _2 15:04:05", line[:15], now.Location())
	if err != nil {
		return time.Time{}, "", false
	}
	t = t.AddDate(now.Year(), 0, 0)
	// A December line read in January belongs to last year.
	if t.After(now.Add(24 * time.Hour)) {
		t = t.AddDate(-1, 0, 0)
	}
	return t, line[16:], true
}

// applyHistory sets each cron job's last run from the log, and records how
// far back the log goes so checks can tell "never ran" from "unknown".
func applyHistory(jobs []job.Job, h history) {
	for i := range jobs {
		j := &jobs[i]
		// Scripts in the periodic folders are started by run-parts, not
		// by cron, so they have no line of their own.
		if j.Source != job.Cron || j.Err != "" || periodicScript(j) {
			continue
		}
		j.HistorySince = h.since
		for _, r := range h.runs {
			if r.cmd != j.Command || (j.RunAs != "" && r.user != j.RunAs) {
				continue
			}
			if r.at.After(j.LastRun) {
				j.LastRun = r.at
			}
		}
	}
}

func periodicScript(j *job.Job) bool {
	for _, p := range periodic {
		if strings.HasPrefix(j.File, p.dir+"/") {
			return true
		}
	}
	return false
}
