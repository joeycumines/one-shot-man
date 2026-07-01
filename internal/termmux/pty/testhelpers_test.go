package pty

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

// buildIdleProgram builds a binary that reads stdin until EOF then exits,
// replacing "cat" in tests that need a long-lived process attached to a PTY.
func buildIdleProgram(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return "cmd.exe"
	}
	return buildProgram(t, `package main

import (
	"io"
	"os"
)

func main() {
	io.Copy(os.Stdout, os.Stdin)
}
`)
}

// buildEchoIdleProgram builds a binary that prints the given text to stdout
// and then reads stdin until EOF, replacing "sh -c 'echo text; exec cat'"
// patterns in tests.
func buildEchoIdleProgram(t *testing.T, text string) string {
	t.Helper()
	return buildProgram(t, fmt.Sprintf(`package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	fmt.Println(%q)
	io.Copy(os.Stdout, os.Stdin)
}
`, text))
}

// buildEchoProgram builds a binary that prints the given text and exits,
// replacing "sh -c 'echo text'" patterns.
func buildEchoProgram(t *testing.T, text string) string {
	t.Helper()
	return buildProgram(t, fmt.Sprintf(`package main

import "fmt"

func main() {
	fmt.Println(%q)
}
`, text))
}

// buildEnvEchoProgram builds a binary that prints the value of the given
// environment variable and exits, replacing "sh -c 'echo $VAR'" patterns.
func buildEnvEchoProgram(t *testing.T, varName string) string {
	t.Helper()
	return buildProgram(t, fmt.Sprintf(`package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println(os.Getenv(%q))
}
`, varName))
}
