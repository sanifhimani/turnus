package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
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
	var jobs []map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &jobs); err != nil {
		t.Fatalf("output is not a JSON array: %v", err)
	}
}
