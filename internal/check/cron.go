package check

import (
	"path/filepath"
	"strings"

	"github.com/sanifhimani/turnus/internal/job"
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
	switch filepath.Dir(j.File) {
	case "/etc/cron.hourly", "/etc/cron.daily", "/etc/cron.weekly", "/etc/cron.monthly":
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
