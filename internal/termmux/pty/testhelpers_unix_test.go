//go:build !windows

package pty

import (
	"testing"
)

// buildChildPidProgram builds a binary that starts a background child process,
// prints its PID as "CHILD_PID=<pid>", then sleeps briefly, replacing
// "sh -c 'sleep 3600 & echo CHILD_PID=$!; sleep 1'" patterns.
func buildChildPidProgram(t *testing.T) string {
	t.Helper()
	return buildProgram(t, `package main

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

func main() {
	cmd := exec.Command("sleep", "3600")
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "start child: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("CHILD_PID=%d\n", cmd.Process.Pid)
	time.Sleep(1 * time.Second)
}
`)
}
