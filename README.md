# turnus

turnus is a terminal UI for the cron jobs, launchd agents and systemd timers on
your machine.

It also reads the tasks you schedule in Claude Desktop, the Codex app and the
Hermes agent. It lists every job in one place and flags the ones that are
broken. It tells you when a job's last run failed or crashed, when its program
no longer exists, when its plist never loaded, and when its schedule can never
run. For each problem, turnus explains how to fix it, usually with a command you
can copy.

turnus only reads your jobs. It never changes them, and it doesn't run in the
background.

![turnus listing failing jobs, opening one, and reading its journal](assets/demo.gif)

## Install

Install turnus with Homebrew.

```sh
brew install sanifhimani/tap/turnus
```

You can also install it with Go 1.26 or later.

```sh
go install github.com/sanifhimani/turnus/cmd/turnus@latest
```

## Usage

Run `turnus` to browse your jobs, or `turnus ls` to print them as a table.

```sh
turnus             # open the browser
turnus ls          # print the table
turnus ls --json   # print JSON
turnus ls --check  # exit with status 1 if any job is failing
turnus -a          # include jobs that ship with the OS
```

`--check` lets you run turnus from CI or from cron as a watchdog. turnus exits
with status 0 when no job is failing, 1 when at least one is, and 2 when it hits
an error. Warnings don't change the exit status.

`--json` prints an object with a `version` number, the `jobs`, and any `errors`
from schedulers turnus couldn't read. The version changes only when a field is
removed or changes meaning.

The browser uses these keys.

| Key | Action |
|---|---|
| `j` `k` or arrows | Move |
| `/` | Filter |
| `enter` | Show the job's details |
| `e` | Edit the job's file, or run `crontab -e` for your crontab |
| `l` | Open the job's log, or `journalctl` for a systemd timer |
| `c` | Copy the fix |
| `a` | Show or hide the OS's own jobs |
| `q` | Quit |

## Checks

turnus runs these checks on every job.

| Check | cron | launchd | systemd |
|---|:-:|:-:|:-:|
| The last run failed or crashed | | ✓ | ✓ |
| The program is missing or not executable | ✓ | ✓ | ✓ |
| The log folder is missing, so the job can't start | ✓ | ✓ | |
| The job isn't loaded or the timer isn't started | | ✓ | ✓ |
| The timer starts a service that doesn't exist | | | ✓ |
| The plist is invalid | | ✓ | |
| The schedule never matches a real date | ✓ | ✓ | ✓ |
| The job runs between 1 and 3 a.m., when daylight saving can skip it | ✓ | ✓ | ✓ |
| The scheduler skips runs missed while the machine sleeps or is off | macOS | `StartInterval` | Without `Persistent=true` |
| The job reads `~/Documents`, which cron can't do without Full Disk Access | macOS | | |
| The job's output goes nowhere | ✓ | | |

For tasks that AI agents schedule, turnus flags a Claude Desktop task that the
app skipped and says why, such as a folder that no longer exists or a signed-out
account. It flags a Hermes job whose last run failed. It also points out that
these tasks run only while the app or service that schedules them is running,
and it warns you when Claude Desktop is closed.

launchd returns exit code 78 for both a missing program and a missing log
folder, and systemd returns 203 for any program it can't start. turnus checks
the files to tell you which problem caused the failure.

## How it works

turnus reads cron jobs from your crontab, `/etc/crontab`, `/etc/cron.d` and the
`/etc/cron.{hourly,daily,weekly,monthly}` folders. It reads launchd jobs from
the LaunchAgents and LaunchDaemons folders and gets their status from
`launchctl print`. It reads systemd timers for both the system and your user
and gets their status from `systemctl show`.

turnus reads Claude Desktop's scheduled tasks from its folder in
`~/Library/Application Support/Claude`, Codex automations from
`~/.codex/automations`, and Hermes jobs from `~/.hermes/cron/jobs.json`. Claude
Desktop and Codex don't document these files, so turnus reads them carefully and
tells you when it finds a format it doesn't know.

cron keeps no history of past runs, so turnus can't tell you whether a cron
job's last run succeeded.

turnus runs on macOS and Linux.

## License

turnus is released under the [MIT License](LICENSE).
