// Command turnus shows every scheduled job on this machine and what is wrong
// with it.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"golang.org/x/term"

	"github.com/sanifhimani/turnus/internal/check"
	"github.com/sanifhimani/turnus/internal/discover"
	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/render"
	"github.com/sanifhimani/turnus/internal/style"
	"github.com/sanifhimani/turnus/internal/ui"
)

// version is set by release builds. Builds from go install fall back to the
// module version Go records in the binary.
var version = "dev"

func versionString() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return version
}

const usage = `turnus shows every scheduled job on this machine and what is wrong with it.

Usage:
  turnus [flags]       browse jobs interactively
  turnus ls [flags]    print jobs as a table

Flags:
  -a, --all      include jobs that ship with the OS
      --json     print jobs as JSON
  -h, --help     show this help
  -v, --version  show the version
`

type options struct {
	ls   bool
	all  bool
	json bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var opts options
	for _, a := range args {
		switch a {
		case "ls":
			if opts.ls {
				return fail(stderr, "unexpected argument %q", a)
			}
			opts.ls = true
		case "-a", "--all":
			opts.all = true
		case "--json":
			opts.json = true
		case "-h", "--help", "help":
			fmt.Fprint(stdout, usage)
			return 0
		case "-v", "--version", "version":
			fmt.Fprintln(stdout, "turnus", versionString())
			return 0
		default:
			return fail(stderr, "unknown argument %q", a)
		}
	}
	if !opts.ls && !opts.json && interactive() {
		return browse(opts, stderr)
	}
	return list(opts, stdout, stderr)
}

// load finds and checks jobs. A source that fails returns an error, and the
// jobs from the other sources are still returned.
func load(all bool) ([]job.Job, []error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jobs, errs := discover.Discover(ctx, discover.Options{Builtin: all})
	check.Run(jobs, check.Local())
	return jobs, errs
}

func report(w io.Writer, errs []error) {
	for _, err := range errs {
		fmt.Fprintln(w, "turnus:", err)
	}
}

func browse(opts options, stderr io.Writer) int {
	jobs, errs := load(opts.all)
	// Errors are printed once, before the screen is taken over.
	report(stderr, errs)
	reload := func(all bool) []job.Job {
		jobs, _ := load(all)
		return jobs
	}

	m := ui.New(reload, jobs, opts.all, style.For(os.Stdout))
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Fprintln(stderr, "turnus:", err)
		return 1
	}
	return 0
}

func list(opts options, stdout, stderr io.Writer) int {
	jobs, errs := load(opts.all)
	report(stderr, errs)

	var err error
	if opts.json {
		err = render.JSON(stdout, jobs, errs)
	} else {
		err = render.Table(stdout, jobs, render.Options{
			Width: termWidth(),
			Now:   time.Now(),
			Style: style.For(os.Stdout),
		})
	}
	if err != nil {
		fmt.Fprintln(stderr, "turnus:", err)
		return 1
	}
	return 0
}

func interactive() bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
}

func termWidth() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		return 0
	}
	return w
}

func fail(w io.Writer, format string, a ...any) int {
	fmt.Fprintf(w, "turnus: "+format+"\n", a...)
	fmt.Fprintln(w, "Run 'turnus --help' for usage.")
	return 2
}
