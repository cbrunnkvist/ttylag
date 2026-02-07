//go:build !windows
// +build !windows

package main

import (
	"bytes"
	"os/exec"
	"testing"
)

func TestPipedOutputNoCRLF(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "--", "echo", "hello")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = nil

	if err := cmd.Run(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	output := stdout.Bytes()
	if len(output) == 0 {
		t.Fatal("no output received")
	}

	if bytes.Contains(output, []byte("\r\n")) {
		t.Errorf("output contains CR+LF (0d 0a), expected only LF; got: %q", output)
	}

	if !bytes.Contains(output, []byte("\n")) {
		t.Errorf("output missing LF; got: %q", output)
	}
}
