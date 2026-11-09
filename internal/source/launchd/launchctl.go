package launchd

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

// service is launchd's view of a loaded job.
type service struct {
	pid      int
	lastExit *int
}

// domain is the parsed output of `launchctl print <domain>`.
type domain struct {
	services map[string]service
	disabled map[string]bool
}

func printDomain(ctx context.Context, name string) (domain, error) {
	out, err := exec.CommandContext(ctx, "launchctl", "print", name).Output()
	if err != nil {
		return domain{}, err
	}
	return parseDomain(strings.NewReader(string(out))), nil
}

// parseDomain reads the "services" and "disabled services" sections. Service
// lines look like "<pid> <last exit> <label>", where the exit status is "-"
// when the job has not exited yet.
func parseDomain(r io.Reader) domain {
	d := domain{services: map[string]service{}, disabled: map[string]bool{}}
	var section string

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "\t") && !strings.HasPrefix(line, "\t\t") {
			section = strings.TrimSpace(line)
			continue
		}
		switch section {
		case "services = {":
			f := strings.Fields(line)
			if len(f) != 3 {
				continue
			}
			pid, _ := strconv.Atoi(f[0])
			s := service{pid: pid}
			if code, err := strconv.Atoi(f[1]); err == nil {
				s.lastExit = &code
			}
			d.services[f[2]] = s
		case "disabled services = {":
			label, state, ok := strings.Cut(strings.TrimSpace(line), " => ")
			if ok {
				d.disabled[strings.Trim(label, `"`)] = state == "disabled"
			}
		}
	}
	return d
}
