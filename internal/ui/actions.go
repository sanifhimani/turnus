package ui

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/sanifhimani/turnus/internal/job"
)

// action handles keys that work in both the list and the detail view.
func (m *model) action(k string) tea.Cmd {
	j := m.selected()
	if j == nil {
		return nil
	}
	switch k {
	case "e":
		return m.edit(j)
	case "l":
		return m.openLog(j)
	case "c":
		return m.copyFix(j)
	}
	return nil
}

func (m *model) edit(j *job.Job) tea.Cmd {
	// A user crontab must go through crontab -e, which installs the result.
	if j.Source == job.Cron && j.Scope == job.User {
		return run(exec.Command("crontab", "-e"))
	}
	if j.File == "" {
		return m.flash("This job has no file to edit")
	}
	args := editor()
	if j.Line > 0 && supportsLine(args[0]) {
		args = append(args, "+"+strconv.Itoa(j.Line))
	}
	args = append(args, j.File)
	return run(exec.Command(args[0], args[1:]...))
}

func (m *model) openLog(j *job.Job) tea.Cmd {
	if j.LogPath == "" {
		return m.flash("This job doesn't write a log file")
	}
	if _, err := os.Stat(j.LogPath); errors.Is(err, os.ErrNotExist) {
		return m.flash("No log yet at " + tilde(j.LogPath))
	}
	pager := strings.Fields(os.Getenv("PAGER"))
	if len(pager) == 0 {
		pager = []string{"less"}
	}
	if filepath.Base(pager[0]) == "less" {
		pager = append(pager, "+G") // start at the newest lines
	}
	pager = append(pager, j.LogPath)
	return run(exec.Command(pager[0], pager[1:]...))
}

func (m *model) copyFix(j *job.Job) tea.Cmd {
	var fix string
	for _, p := range j.Problems {
		if p.Fix != "" {
			fix = p.Fix
			break
		}
	}
	if fix == "" {
		return m.flash("Nothing to fix")
	}
	// Terminal.app ignores the OSC 52 clipboard sequence, so use pbcopy on
	// macOS.
	if runtime.GOOS == "darwin" {
		cmd := exec.Command("pbcopy")
		cmd.Stdin = bytes.NewBufferString(fix)
		if err := cmd.Run(); err != nil {
			return m.flash("Couldn't copy: " + err.Error())
		}
		return m.flash("Copied: " + fix)
	}
	return tea.Batch(tea.SetClipboard(fix), m.flash("Copied: "+fix))
}

// run hands the terminal to an interactive program and reloads after.
func run(cmd *exec.Cmd) tea.Cmd {
	return tea.ExecProcess(cmd, func(err error) tea.Msg { return execDoneMsg{err: err} })
}

func editor() []string {
	for _, env := range []string{"VISUAL", "EDITOR"} {
		if f := strings.Fields(os.Getenv(env)); len(f) > 0 {
			return f
		}
	}
	return []string{"vi"}
}

// supportsLine reports whether the editor accepts "+N" to open at a line.
func supportsLine(editor string) bool {
	switch filepath.Base(editor) {
	case "vi", "vim", "nvim", "nano", "emacs", "micro", "hx", "kak":
		return true
	}
	return false
}
