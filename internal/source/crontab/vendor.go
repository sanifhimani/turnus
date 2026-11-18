package crontab

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/sanifhimani/turnus/internal/job"
)

// dpkgStatus lists every installed Debian package, including the config
// files each one installed under /etc.
const dpkgStatus = "/var/lib/dpkg/status"

// packageFiles returns which of the given cron files an OS package installed.
// It asks whichever package manager the machine has: dpkg on Debian and
// Ubuntu, rpm on Fedora and Red Hat, pacman on Arch, apk on Alpine. It is
// empty when there's none, as on macOS.
func packageFiles(ctx context.Context, files []string) map[string]bool {
	if len(files) == 0 {
		return map[string]bool{}
	}
	if _, err := os.Stat(dpkgStatus); err == nil {
		return dpkgFiles(dpkgStatus)
	}
	switch {
	case has("rpm"):
		// Lists every file of the packages that own any of these. A file
		// no package owns prints a "not owned" line instead, which matches
		// nothing.
		out := query(ctx, "rpm", append([]string{"-qf", "--qf", "[%{FILENAMES}\n]"}, files...)...)
		return listed(out, files)
	case has("pacman"):
		names := query(ctx, "pacman", append([]string{"-Qqo"}, files...)...)
		pkgs := strings.Fields(string(names))
		if len(pkgs) == 0 {
			return map[string]bool{}
		}
		return listed(query(ctx, "pacman", append([]string{"-Qlq"}, pkgs...)...), files)
	case has("apk"):
		return apkOwned(query(ctx, "apk", append([]string{"info", "--who-owns"}, files...)...))
	}
	return map[string]bool{}
}

// cronFiles returns the files under /etc/cron* that jobs came from, the
// ones a package may have installed. /etc/crontab is left out: admins add
// lines to it, so markBuiltin judges it line by line.
func cronFiles(jobs []job.Job) []string {
	var files []string
	for _, j := range jobs {
		if strings.HasPrefix(j.File, "/etc/cron") && j.File != "/etc/crontab" && !slices.Contains(files, j.File) {
			files = append(files, j.File)
		}
	}
	return files
}

func has(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// query returns a command's output. Package managers exit non-zero when any
// file is unowned, and still print the rest, so the error is ignored.
func query(ctx context.Context, name string, args ...string) []byte {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &out
	_ = cmd.Run()
	return out.Bytes()
}

// listed returns which of the files appear as a line in out.
func listed(out []byte, files []string) map[string]bool {
	owned := map[string]bool{}
	for line := range strings.Lines(string(out)) {
		if f := strings.TrimSpace(line); slices.Contains(files, f) {
			owned[f] = true
		}
	}
	return owned
}

// apkOwned reads lines such as "/etc/cron.d/x is owned by pkg-1.0-r0".
func apkOwned(out []byte) map[string]bool {
	owned := map[string]bool{}
	for line := range strings.Lines(string(out)) {
		if f, _, ok := strings.Cut(line, " is owned by "); ok {
			owned[strings.TrimSpace(f)] = true
		}
	}
	return owned
}

// dpkgFiles returns the cron files installed by packages, read from the
// dpkg database.
func dpkgFiles(path string) map[string]bool {
	files := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		return files
	}
	defer f.Close()
	return dpkgConffiles(f)
}

func dpkgConffiles(r io.Reader) map[string]bool {
	files := map[string]bool{}
	sc := bufio.NewScanner(r)
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
// lines count. Alpine puts the same kind of line, running the folders under
// /etc/periodic, in root's own crontab.
func markBuiltin(jobs []job.Job, packaged map[string]bool) {
	for i := range jobs {
		j := &jobs[i]
		switch {
		case j.File == "/etc/crontab":
			j.Builtin = strings.Contains(j.Command, "run-parts")
		case packaged[j.File]:
			j.Builtin = true
		case runsSystemFolder(j.Command):
			j.Builtin = true
		}
	}
}

// runsSystemFolder reports whether a command runs one of the OS's periodic
// folders with run-parts, as in "run-parts /etc/periodic/daily".
func runsSystemFolder(cmd string) bool {
	if !strings.Contains(cmd, "run-parts") {
		return false
	}
	for _, f := range strings.Fields(cmd) {
		f = strings.TrimRight(f, ";)}")
		if strings.HasPrefix(f, "/etc/periodic/") || strings.HasPrefix(f, "/etc/cron.") && f != "/etc/cron.d" {
			return true
		}
	}
	return false
}
