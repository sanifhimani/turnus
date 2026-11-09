package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/sanifhimani/turnus/internal/job"
)

func TestJSON(t *testing.T) {
	var buf bytes.Buffer
	jobs := []job.Job{{Name: "backup", Source: job.Cron}}
	if err := JSON(&buf, jobs, []error{errors.New("systemd: systemctl failed")}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version int              `json:"version"`
		Jobs    []map[string]any `json:"jobs"`
		Errors  []string         `json:"errors"`
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || len(got.Jobs) != 1 || got.Jobs[0]["name"] != "backup" || len(got.Errors) != 1 {
		t.Errorf("got %+v", got)
	}
}

func TestJSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, nil, nil); err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"version\": 1,\n  \"jobs\": [],\n  \"errors\": []\n}\n"
	if buf.String() != want {
		t.Errorf("got %q, want %q", buf.String(), want)
	}
}
