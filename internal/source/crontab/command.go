package crontab

import (
	"path/filepath"
	"slices"
	"strings"
	"unicode"
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

// builtins are run by the shell itself, or are keywords that close a compound
// command, so PATH plays no part in finding them.
var builtins = map[string]bool{
	":": true, "[": true, "[[": true, "alias": true, "bg": true, "break": true,
	"builtin": true, "case": true, "cd": true, "command": true, "continue": true,
	"declare": true, "done": true, "echo": true, "esac": true, "eval": true,
	"exit": true, "export": true, "false": true, "fg": true, "fi": true,
	"for": true, "function": true, "getopts": true, "hash": true, "jobs": true,
	"kill": true, "let": true, "local": true, "printf": true, "pwd": true,
	"read": true, "readonly": true, "return": true, "select": true, "set": true,
	"shift": true, "test": true, "times": true, "trap": true, "true": true,
	"type": true, "typeset": true, "ulimit": true, "umask": true,
	"unalias": true, "unset": true, "wait": true, "}": true, ")": true,
}

// prefixes precede the command they run, which is looked up as usual.
var prefixes = map[string]bool{
	"!": true, "{": true, "(": true, "if": true, "then": true, "else": true,
	"elif": true, "while": true, "until": true, "do": true, "exec": true,
	"nohup": true, "time": true,
}

// Shells that read the user's startup files on every run, where PATH is
// often set. Commands they run can't be checked against a known PATH.
var profileShells = map[string]bool{"zsh": true, "fish": true}

// Shells that read startup files only as a login or interactive shell.
var shells = map[string]bool{"sh": true, "bash": true, "dash": true, "ksh": true}

// pathLookups returns the programs a command runs by bare name, which the
// shell finds by searching PATH. It returns nil when the command may change
// PATH before running them: by setting it, by sourcing a file, or by starting
// a shell that reads the user's startup files. Words only the shell can
// resolve, such as $TOOL, are left out.
//
//	cd /srv && node sync.js | gzip  ->  node, gzip
func pathLookups(cmd string) []string {
	if strings.Contains(cmd, "PATH=") || startsProfileShell(shellFields(cmd)) {
		return nil
	}
	var names []string
	for _, seg := range segments(cmd) {
		for part := range strings.SplitSeq(seg, "|") {
			fields := shellFields(part)
			for len(fields) > 0 && (prefixes[fields[0]] || assignment(fields[0])) {
				fields = fields[1:]
			}
			if len(fields) == 0 {
				continue
			}
			name := fields[0]
			switch {
			case name == "." || name == "source":
				return nil
			case builtins[name], strings.ContainsAny(name, "/$`~=*?<>&"), slices.Contains(names, name):
				continue
			}
			names = append(names, name)
			// A non-login shell keeps the PATH it was given, so the
			// script it runs is looked up the same way.
			if shells[name] && len(fields) > 2 && fields[1] == "-c" {
				for _, inner := range pathLookups(fields[2]) {
					if !slices.Contains(names, inner) {
						names = append(names, inner)
					}
				}
			}
		}
	}
	return names
}

// startsProfileShell reports whether the words run zsh or fish, or another
// shell as a login or interactive shell.
func startsProfileShell(fields []string) bool {
	for i, f := range fields {
		base := filepath.Base(f)
		if profileShells[base] {
			return true
		}
		if !shells[base] || i+1 == len(fields) {
			continue
		}
		flag := fields[i+1]
		if flag == "--login" || (strings.HasPrefix(flag, "-") && !strings.HasPrefix(flag, "--") && strings.ContainsAny(flag, "il")) {
			return true
		}
	}
	return false
}

// assignment reports whether a word sets a variable for the command after
// it, as in FOO=1.
func assignment(word string) bool {
	name, _, ok := strings.Cut(word, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if r != '_' && !unicode.IsLetter(r) && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

// beforePercent returns the command up to its first unescaped %. cron turns
// that % into a newline and feeds the rest to the command as input.
func beforePercent(cmd string) string {
	if i := UnescapedPercent(cmd); i >= 0 {
		return cmd[:i]
	}
	return cmd
}

// UnescapedPercent returns the index of the first % not preceded by a
// backslash, or -1.
func UnescapedPercent(cmd string) int {
	for i := 0; i < len(cmd); i++ {
		switch cmd[i] {
		case '\\':
			i++
		case '%':
			return i
		}
	}
	return -1
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
