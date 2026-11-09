// Command turnus shows every scheduled job on this machine and what is wrong
// with it.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/sanifhimani/turnus/internal/check"
	"github.com/sanifhimani/turnus/internal/discover"
	"github.com/sanifhimani/turnus/internal/render"
	"github.com/sanifhimani/turnus/internal/style"
)

// version is set at build time.
var version = "dev"

const usage = `turnus shows every scheduled job on this machine and what is wrong with it.

Usage:
  turnus [ls] [flags]

Flags:
  -a, --all      include jobs that ship with the OS
      --json     print jobs as JSON
  -h, --help     show this help
  -v, --version  show the version
`

type options struct {
	all  bool
	json bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	var opts options
	for i, a := range args {
		switch a {
		case "ls":
			if i != 0 {
				return fail(stderr, "unexpected argument %q", a)
			}
		case "-a", "--all":
			opts.all = true
		case "--json":
			opts.json = true
		case "-h", "--help", "help":
			fmt.Fprint(stdout, usage)
			return 0
		case "-v", "--version", "version":
			fmt.Fprintln(stdout, "turnus", version)
			return 0
		default:
			return fail(stderr, "unknown argument %q", a)
		}
	}
	return list(opts, stdout, stderr)
}

func list(opts options, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	jobs, errs := discover.Discover(ctx, discover.Options{Builtin: opts.all})
	check.Run(jobs, check.Local())
	for _, err := range errs {
		fmt.Fprintln(stderr, "turnus:", err)
	}

	var err error
	if opts.json {
		err = render.JSON(stdout, jobs)
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
