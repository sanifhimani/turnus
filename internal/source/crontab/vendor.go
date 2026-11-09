package crontab

import (
	"bufio"
	"os"
	"strings"

	"github.com/sanifhimani/turnus/internal/job"
)

// dpkgStatus lists every installed Debian package, including the config
// files each one installed under /etc.
const dpkgStatus = "/var/lib/dpkg/status"

// packageFiles returns the cron files installed by packages, read from the
// dpkg database. It is empty on systems without dpkg.
func packageFiles(path string) map[string]bool {
	files := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return files
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		// Conffiles entries look like " /etc/cron.daily/dpkg <md5>".
		line := sc.Text()
		if !strings.HasPrefix(line, " /etc/cron") {
			continue
		}
		if p, _, ok := strings.Cut(line[1:], " "); ok {
			files[p] = true
		}
	}
	return files
}

// markBuiltin flags jobs that came with the OS. /etc/crontab belongs to the
// cron package but admins add lines to it, so only its standard run-parts
// lines count.
func markBuiltin(jobs []job.Job, packaged map[string]bool) {
	for i := range jobs {
		j := &jobs[i]
		switch {
		case j.File == "/etc/crontab":
			j.Builtin = strings.Contains(j.Command, "run-parts")
		case packaged[j.File]:
			j.Builtin = true
		}
	}
}
