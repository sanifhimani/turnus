package check

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/source/crontab"
)

// Folders macOS guards with privacy permissions. cron needs Full Disk Access
// to read them, and fails without saying so.
var protectedFolders = []string{
	"/Desktop", "/Documents", "/Downloads", "/Library/Mobile Documents",
}

func protectedFolder(j *job.Job, env Env) *job.Problem {
	if env.GOOS != "darwin" || j.Source != job.Cron || j.Err != "" || env.Home == "" {
		return nil
	}
	cmd := strings.ReplaceAll(j.Command, "$HOME", env.Home)
	cmd = strings.ReplaceAll(cmd, "~", env.Home)
	for _, f := range protectedFolders {
		if strings.Contains(cmd, env.Home+f) {
			return &job.Problem{
				Code:     "protected-folder",
				Severity: job.Warn,
				Summary:  "Uses " + strings.TrimPrefix(f, "/") + ", which cron can't read without Full Disk Access",
				Fix:      "Give /usr/sbin/cron Full Disk Access in System Settings > Privacy & Security",
			}
		}
	}
	return nil
}

// output reports cron jobs whose output, including error messages, ends up
// somewhere nobody looks.
func output(j *job.Job, env Env) *job.Problem {
	if j.Source != job.Cron || j.Err != "" || j.LogPath != "" {
		return nil
	}
	// Scripts in /etc/cron.daily and friends inherit the output handling of
	// the run-parts line that runs the folder.
	if periodicDir(j.File) {
		return nil
	}
	logDir := "~"
	switch {
	case env.GOOS == "darwin":
		logDir = "~/Library/Logs"
	case j.Scope == job.System:
		logDir = "/var/log"
	}
	fix := "Send output to a log by adding >> " + shellPath(logDir+"/"+j.Name+".log") + " 2>&1"

	var summary string
	mailto, set := j.Env["MAILTO"]
	switch {
	case discardsStdout(j.Command):
		summary = "Output is thrown away, so errors leave no trace"
	case set && mailto == "":
		summary = "Output is discarded because MAILTO is empty"
	case set:
		return nil
	case env.HasMail != nil && !env.HasMail():
		summary = "Output is discarded because no mail server is installed"
	default:
		summary = "Output goes to local mail, which few people read"
	}
	return &job.Problem{Code: "output", Severity: job.Info, Summary: summary, Fix: fix}
}

func discardsStdout(cmd string) bool {
	fields := strings.Fields(cmd)
	for i, f := range fields {
		switch f {
		case ">/dev/null", ">>/dev/null", "1>/dev/null", "&>/dev/null":
			return true
		case ">", ">>", "1>", "&>":
			if i+1 < len(fields) && fields[i+1] == "/dev/null" {
				return true
			}
		}
	}
	return false
}

// percent flags a % in a cron command. cron turns the first unescaped % into
// a newline and passes the rest as input, so "date +%Y" breaks.
func percent(j *job.Job, env Env) *job.Problem {
	if j.Source != job.Cron || j.Err != "" || crontab.UnescapedPercent(j.Command) < 0 {
		return nil
	}
	return &job.Problem{
		Code:     "unescaped-percent",
		Severity: job.Warn,
		Summary:  "cron ends the command at the first %, so the rest becomes its input",
		Fix:      `Escape each % as \% if it's part of the command`,
	}
}

// runPartsName matches the script names Debian's run-parts accepts. It skips
// anything else, such as backup.sh, without saying so.
var runPartsName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func skippedByRunParts(j *job.Job, env Env) *job.Problem {
	if !env.DebianRunParts || j.Source != job.Cron || !periodicDir(j.File) {
		return nil
	}
	name := filepath.Base(j.File)
	if runPartsName.MatchString(name) {
		return nil
	}
	fix := "Rename it using only letters, digits, - and _"
	if base := strings.TrimSuffix(name, filepath.Ext(name)); runPartsName.MatchString(base) {
		fix = "Rename it to " + base
	}
	return &job.Problem{
		Code:     "skipped-by-run-parts",
		Severity: job.Fail,
		Summary:  "run-parts skips this script because of the characters in its name, so it never runs",
		Fix:      fix,
	}
}

func periodicDir(path string) bool {
	switch filepath.Dir(path) {
	case "/etc/cron.hourly", "/etc/cron.daily", "/etc/cron.weekly", "/etc/cron.monthly":
		return true
	}
	return false
}
