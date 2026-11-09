// Package discover collects jobs from every scheduler on the machine.
package discover

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/sanifhimani/turnus/internal/job"
	"github.com/sanifhimani/turnus/internal/source/crontab"
	"github.com/sanifhimani/turnus/internal/source/launchd"
	"github.com/sanifhimani/turnus/internal/source/systemd"
)

// Options controls what Discover returns.
type Options struct {
	// Builtin includes jobs that ship with the operating system.
	Builtin bool
}

type source struct {
	name string
	find func(context.Context, time.Time) ([]job.Job, error)
}

// Discover queries all sources concurrently. A failing source does not stop
// the others; its error is returned alongside the jobs that were found.
func Discover(ctx context.Context, opts Options) ([]job.Job, []error) {
	now := time.Now()
	sources := []source{
		{"cron", crontab.Discover},
		{"launchd", func(ctx context.Context, now time.Time) ([]job.Job, error) {
			return launchd.Discover(ctx, now, opts.Builtin)
		}},
		{"systemd", func(ctx context.Context, now time.Time) ([]job.Job, error) {
			return systemd.Discover(ctx, now, opts.Builtin)
		}},
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results = make([][]job.Job, len(sources))
		errs    []error
	)
	for i, s := range sources {
		wg.Go(func() {
			jobs, err := s.find(ctx, now)
			if err != nil {
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s: %w", s.name, err))
				mu.Unlock()
			}
			results[i] = jobs
		})
	}
	wg.Wait()

	jobs := slices.Concat(results...)
	slices.SortStableFunc(jobs, compare)
	return jobs, errs
}

// compare puts scheduled jobs before services, then sorts by name.
func compare(a, b job.Job) int {
	if a.Kind != b.Kind {
		if a.Kind == job.Scheduled {
			return -1
		}
		return 1
	}
	return cmp.Compare(a.Name, b.Name)
}
