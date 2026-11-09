package render

import (
	"encoding/json"
	"io"

	"github.com/sanifhimani/turnus/internal/job"
)

// SchemaVersion is the version of the JSON output. It changes only when a
// field is removed or changes meaning, never when one is added.
const SchemaVersion = 1

type report struct {
	Version int       `json:"version"`
	Jobs    []job.Job `json:"jobs"`
	Errors  []string  `json:"errors"`
}

// JSON writes jobs, and the errors from any source that couldn't be read, as
// one versioned object. Empty lists are written as [] rather than null, so
// scripts can always iterate over them.
func JSON(w io.Writer, jobs []job.Job, errs []error) error {
	r := report{Version: SchemaVersion, Jobs: jobs, Errors: make([]string, 0, len(errs))}
	if r.Jobs == nil {
		r.Jobs = []job.Job{}
	}
	for _, err := range errs {
		r.Errors = append(r.Errors, err.Error())
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
