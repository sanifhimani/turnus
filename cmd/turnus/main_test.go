package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sanifhimani/turnus/internal/job"
)

func TestRunArgs(t *testing.T) {
	tests := []struct {
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{args: []string{"--help"}, code: 0, stdout: "Usage:"},
		{args: []string{"-h"}, code: 0, stdout: "Usage:"},
		{args: []string{"--version"}, code: 0, stdout: "turnus dev"},
		{args: []string{"--nope"}, code: 2, stderr: `unknown argument "--nope"`},
		{args: []string{"ls", "ls"}, code: 2, stderr: `unexpected argument "ls"`},
	}
	for _, tt := range tests {
		var stdout, stderr bytes.Buffer
		code := run(tt.args, &stdout, &stderr)
		if code != tt.code {
			t.Errorf("run(%q) = %d, want %d", tt.args, code, tt.code)
		}
		if !strings.Contains(stdout.String(), tt.stdout) {
			t.Errorf("run(%q) stdout = %q, want it to contain %q", tt.args, stdout.String(), tt.stdout)
		}
		if !strings.Contains(stderr.String(), tt.stderr) {
			t.Errorf("run(%q) stderr = %q, want it to contain %q", tt.args, stderr.String(), tt.stderr)
		}
	}
}

func TestRunJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	// Flags may come before or after the command.
	if code := run([]string{"--json", "-a", "ls"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	var out struct {
		Version int              `json:"version"`
		Jobs    []map[string]any `json:"jobs"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil || out.Version != 1 || out.Jobs == nil {
		t.Fatalf("unexpected output: %v\n%s", err, stdout.String())
	}
}

func TestFailing(t *testing.T) {
	warn := []job.Job{{Problems: []job.Problem{{Severity: job.Warn}}}, {}}
	if failing(warn) || failing(nil) {
		t.Error("warnings and empty lists should not count as failing")
	}
	fail := append(warn, job.Job{Problems: []job.Problem{{Severity: job.Fail}}})
	if !failing(fail) {
		t.Error("a job with a failure should count as failing")
	}
}
