//go:build !windows
// +build !windows

package main

import (
	"github.com/creack/pty"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// TestNoRawModeWhenStdoutNotTTY verifies that ttylag does NOT enable raw mode
// on its stdin terminal when stdout is not a TTY, even if stdin is a TTY.
func TestNoRawModeWhenStdoutNotTTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test not applicable on Windows")
	}

	// Build the ttylag binary from the repository. Prefer common layouts.
	// If cmd/ttylag exists, build from there, else build from root.
	var mainPkg string
	if _, err := os.Stat(filepath.Join("cmd", "ttylag", "main.go")); err == nil {
		mainPkg = "./cmd/ttylag"
	} else {
		mainPkg = "./"
	}

	binPath := filepath.Join(t.TempDir(), "ttylag_bin")
	buildCmd := exec.Command("go", "build", "-o", binPath, mainPkg)
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build ttylag binary: %v\n%s", err, string(out))
	}

	// Open a PTY pair for stdin/stdout to interact with ttylag.
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	// Ensure resources are cleaned up
	defer master.Close()
	defer slave.Close()

	// Create a non-tty stdout pipe for the child process
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	defer stdoutR.Close()
	defer stdoutW.Close()

	// Prepare the ttylag process: run a short-lived command (sleep 0.2s)
	// so it stays alive briefly.
	cmd := exec.Command(binPath, "sleep", "0.2")
	cmd.Stdin = slave
	cmd.Stdout = stdoutW
	cmd.Stderr = stdoutW

	// Gate Setctty usage to Linux only to avoid macOS-specific failures
	if runtime.GOOS == "linux" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: int(slave.Fd())}
	} else {
		// macOS and others: omit Setctty; rely on PTY slave to present a terminal
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start ttylag: %v", err)
	}

	// Short delay to let ttylag initialize and potentially modify termios.
	time.Sleep(50 * time.Millisecond)
	// Query the termios settings of the PTY slave via ioctl (no external commands).
	const maxTries = 5
	var lastTerm unix.Termios
	got := false
	for i := 0; i < maxTries; i++ {
		// small delay to allow ttylag to run and potentially modify termios
		time.Sleep(20 * time.Millisecond)
		termPtr, err := unix.IoctlGetTermios(int(slave.Fd()), ioctlGetTermios)
		if err != nil {
			continue
		}
		lastTerm = *termPtr
		if (lastTerm.Lflag&unix.ICANON) != 0 && (lastTerm.Lflag&unix.ECHO) != 0 {
			got = true
			break
		}
	}
	if !got {
		t.Fatalf("expected ICANON and ECHO to be enabled on PTY slave termios; Lflag=%#x", lastTerm.Lflag)
	}

	// Wait for the child to exit, with a timeout to keep test stable.
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("tty lag process exited with error: %v", err)
		}
	case <-time.After(3 * time.Second):
		// If the process hangs, kill it and fail.
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		t.Fatalf("timeout waiting for ttylag process to finish; test unstable")
	}

	// Cleanup: rely on defers for master/slave; avoid double-close.
	// Signal end of child writes and drain any remaining stdout before test exits.
	_ = stdoutW.Close()
	_, _ = io.Copy(io.Discard, stdoutR)
	_ = stdoutR.Close()
}
