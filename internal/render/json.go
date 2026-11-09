package render

import (
	"encoding/json"
	"io"

	"github.com/sanifhimani/turnus/internal/job"
)

// JSON writes jobs as an indented array. An empty result is [] rather than
// null, so scripts can always iterate over it.
func JSON(w io.Writer, jobs []job.Job) error {
	if jobs == nil {
		jobs = []job.Job{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jobs)
}
