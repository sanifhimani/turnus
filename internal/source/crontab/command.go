package crontab

import (
	"path/filepath"
	"strings"
)

var interpreters = map[string]bool{
	"sh": true, "bash": true, "zsh": true,
	"python": true, "python3": true, "node": true, "bun": true, "deno": true,
	"php": true, "ruby": true, "perl": true,
}

// commandName picks a short name for a shell command and, when it can tell,
// the program or script that the command runs. interpreted is set when that
// script is passed to an interpreter such as sh or python.
//
//	cd /srv/app && ./bin/backup.sh  ->  backup.sh, /srv/app/bin/backup.sh
//	/usr/bin/python3 /opt/sync.py   ->  sync.py, /opt/sync.py
func commandName(cmd string) (name, program string, interpreted bool) {
	var dir string
	for _, seg := range segments(cmd) {
		fields := shellFields(seg)
		for len(fields) > 0 && (fields[0] == "{" || fields[0] == "(") {
			fields = fields[1:]
		}
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "cd":
			if len(fields) > 1 {
				dir = fields[1]
			}
			continue
		case "test", "[", "[[":
			continue
		}
		// Debian's /etc/crontab runs whole folders: name the folder.
		if filepath.Base(fields[0]) == "run-parts" {
			return filepath.Base(strings.TrimRight(fields[len(fields)-1], ";}")), "", false
		}

		i := 0
		for i < len(fields) && strings.Contains(fields[i], "=") {
			i++
		}
		if i == len(fields) {
			continue
		}
		prog := fields[i]
		if interpreters[filepath.Base(prog)] && i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "-") {
			prog = fields[i+1]
			interpreted = true
		}
		prog = strings.TrimRight(prog, ";")

		switch {
		case filepath.IsAbs(prog):
			program = prog
		case strings.Contains(prog, "/") && filepath.IsAbs(dir):
			program = filepath.Join(dir, prog)
		}
		return filepath.Base(prog), program, interpreted
	}
	return strings.TrimSpace(cmd), "", false
}

// segments splits a command line on the shell's list operators.
func segments(cmd string) []string {
	return strings.FieldsFunc(strings.NewReplacer("&&", "\x00", "||", "\x00", ";", "\x00").Replace(cmd), func(r rune) bool {
		return r == 0
	})
}

// logPath returns the file that stdout is appended or written to, if any.
func logPath(cmd string) string {
	fields := shellFields(cmd)
	for i, f := range fields {
		var target string
		switch {
		case f == ">" || f == ">>" || f == "1>" || f == "1>>":
			if i+1 < len(fields) {
				target = fields[i+1]
			}
		case strings.HasPrefix(f, ">>"):
			target = f[2:]
		case strings.HasPrefix(f, ">") && !strings.HasPrefix(f, ">&"):
			target = f[1:]
		}
		if filepath.IsAbs(target) && target != "/dev/null" {
			return target
		}
	}
	return ""
}

// shellFields splits a command into words the way a POSIX shell would,
// honoring single quotes, double quotes, and backslash escapes. It does not
// expand variables.
func shellFields(s string) []string {
	var (
		fields []string
		word   strings.Builder
		inWord bool
		quote  rune
		escape bool
	)
	for _, r := range s {
		switch {
		case escape:
			word.WriteRune(r)
			escape = false
		case r == '\\' && quote != '\'':
			escape, inWord = true, true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inWord = r, true
		case r == ' ' || r == '\t':
			if inWord {
				fields = append(fields, word.String())
				word.Reset()
				inWord = false
			}
		default:
			word.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		fields = append(fields, word.String())
	}
	return fields
}
