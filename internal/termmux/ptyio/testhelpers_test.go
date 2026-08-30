package ptyio

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// buildProgram compiles a Go source string into a binary and returns its path.
func buildProgram(t *testing.T, src string) string {
	t.Helper()
	if testing.Short() {
		t.Skip("spawns process to build test helper")
	}
	dir := t.TempDir()
	sourceFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(sourceFile, []byte(src), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}
	binName := "testprog"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	bin := filepath.Join(dir, binName)
	cmd := exec.Command("go", "build", "-o", bin, sourceFile)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build helper: %v\n%s", err, stderr.String())
	}
	return bin
}

// buildEchoSleepProgram builds a binary that prints the given text, sleeps
// briefly, then exits, replacing "sh -c 'echo text && sleep 0.1'" patterns.
func buildEchoSleepProgram(t *testing.T, text string) string {
	t.Helper()
	return buildProgram(t, fmt.Sprintf(`package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println(%q)
	time.Sleep(100 * time.Millisecond)
}
`, text))
}
