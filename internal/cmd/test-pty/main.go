//go:build darwin

// Package main provides a PTY-based test runner that checks terminal state before/after tests.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
)

// termios represents terminal I/O settings
type termios struct {
	Iflag  uint64
	Oflag  uint64
	Cflag  uint64
	Lflag  uint64
	Cc     [20]byte
	Ispeed uint64
	Ospeed uint64
}

var (
	isolateTests = flag.Bool("isolate", false, "Run each test individually to isolate TTY state corruption")
	showOutput   = flag.Bool("output", false, "Show full test output")
)

func getTermios(fd int) (*termios, error) {
	var t termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&t)))
	if errno != 0 {
		return nil, errno
	}
	return &t, nil
}

func termiosEqual(a, b *termios) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Iflag == b.Iflag &&
		a.Oflag == b.Oflag &&
		a.Cflag == b.Cflag &&
		a.Lflag == b.Lflag &&
		a.Cc == b.Cc
}

func formatTermios(t *termios) string {
	if t == nil {
		return "<nil>"
	}
	var flags []string

	// Input flags
	if t.Iflag&0x0001 != 0 {
		flags = append(flags, "IGNBRK")
	}
	if t.Iflag&0x0002 != 0 {
		flags = append(flags, "BRKINT")
	}
	if t.Iflag&0x0020 != 0 {
		flags = append(flags, "INLCR")
	}
	if t.Iflag&0x0040 != 0 {
		flags = append(flags, "IGNCR")
	}
	if t.Iflag&0x0080 != 0 {
		flags = append(flags, "ICRNL")
	}

	// Local flags
	if t.Lflag&0x0008 != 0 {
		flags = append(flags, "ECHO")
	}
	if t.Lflag&0x0002 != 0 {
		flags = append(flags, "ICANON")
	}
	if t.Lflag&0x0001 != 0 {
		flags = append(flags, "ISIG")
	}
	if t.Lflag&0x8000 != 0 {
		flags = append(flags, "IEXTEN")
	}

	return fmt.Sprintf("iflag=0x%x oflag=0x%x cflag=0x%x lflag=0x%x [%s]",
		t.Iflag, t.Oflag, t.Cflag, t.Lflag, strings.Join(flags, ","))
}

func listTests(pkg string) ([]string, error) {
	cmd := exec.Command("go", "test", "-list", ".", pkg)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list tests: %w", err)
	}
	var tests []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "ok") && !strings.HasPrefix(line, "?") {
			tests = append(tests, line)
		}
	}
	return tests, nil
}

func runTestInPTY(pkg string, testName string) (output string, beforeState, afterState *termios, err error) {
	// Create command
	args := []string{"test", "-v", "-timeout=2m", "-count=1"}
	if testName != "" {
		args = append(args, "-run", "^"+regexp.QuoteMeta(testName)+"$")
	}
	args = append(args, pkg)
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	// Start with PTY
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return "", nil, nil, fmt.Errorf("failed to start PTY: %w", err)
	}
	defer ptmx.Close()

	// Get initial termios state
	beforeState, _ = getTermios(int(ptmx.Fd()))

	// Read all output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(ptmx)

	// Wait for command
	_ = cmd.Wait()

	// Get final termios state
	afterState, _ = getTermios(int(ptmx.Fd()))

	return buf.String(), beforeState, afterState, nil
}

func main() {
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Println("Usage: test-pty [flags] <package> [<package>...]")
		fmt.Println("Flags:")
		flag.PrintDefaults()
		fmt.Println("\nExample: test-pty -isolate ./internal/command")
		os.Exit(1)
	}

	packages := flag.Args()
	failures := 0
	stateCorruptions := 0

	for _, pkg := range packages {
		if *isolateTests {
			// Run each test individually
			tests, err := listTests(pkg)
			if err != nil {
				fmt.Printf("❌ Failed to list tests for %s: %v\n", pkg, err)
				failures++
				continue
			}
			fmt.Printf("\n════════════════════════════════════════════════════════════════\n")
			fmt.Printf("ISOLATING %d TESTS IN: %s\n", len(tests), pkg)
			fmt.Printf("════════════════════════════════════════════════════════════════\n")

			for _, testName := range tests {
				output, before, after, err := runTestInPTY(pkg, testName)
				if err != nil {
					fmt.Printf("  ❌ %s: ERROR: %v\n", testName, err)
					failures++
					continue
				}

				corrupted := !termiosEqual(before, after)
				// Check for real test failure: "FAIL\t" at the end of output is the Go test failure line
				// Avoid false positives from "FAIL" appearing in test output/error messages
				lines := strings.Split(output, "\n")
				failed := false
				for i := len(lines) - 1; i >= 0 && i >= len(lines)-5; i-- {
					line := strings.TrimSpace(lines[i])
					if strings.HasPrefix(line, "FAIL\t") || line == "FAIL" {
						failed = true
						break
					}
				}

				status := "✅"
				if failed {
					status = "❌"
					failures++
				}
				if corrupted {
					status = "⚠️  TTY CORRUPTED"
					stateCorruptions++
				}

				fmt.Printf("  %s %s\n", status, testName)
				if corrupted {
					fmt.Printf("       BEFORE: %s\n", formatTermios(before))
					fmt.Printf("       AFTER:  %s\n", formatTermios(after))
				}
				if (failed || corrupted) && *showOutput {
					lines := strings.Split(output, "\n")
					start := len(lines) - 30
					if start < 0 {
						start = 0
					}
					for _, line := range lines[start:] {
						fmt.Printf("       %q\n", line)
					}
				}
			}
		} else {
			// Run package as a whole
			fmt.Printf("\n════════════════════════════════════════════════════════════════\n")
			fmt.Printf("TESTING: %s\n", pkg)
			fmt.Printf("════════════════════════════════════════════════════════════════\n")

			output, before, after, err := runTestInPTY(pkg, "")
			if err != nil {
				fmt.Printf("❌ ERROR: %v\n", err)
				failures++
				continue
			}

			// Check for raw escape sequences or corruption in output
			hasRawEscapes := strings.Contains(output, "\x1b[") && strings.Contains(output, "\x1b[?")
			if hasRawEscapes {
				fmt.Printf("⚠️  WARNING: Output contains raw escape sequences\n")
			}

			// Check for test failures - look for real FAIL line at end, not in test output
			lines := strings.Split(output, "\n")
			failed := false
			for i := len(lines) - 1; i >= 0 && i >= len(lines)-10; i-- {
				line := strings.TrimSpace(lines[i])
				if strings.HasPrefix(line, "FAIL\t") && !strings.Contains(line, "[no test files]") {
					failed = true
					break
				}
				if line == "FAIL" {
					failed = true
					break
				}
			}

			if failed {
				fmt.Printf("❌ TEST FAILED\n")
				// Print last 50 lines of output
				start := len(lines) - 50
				if start < 0 {
					start = 0
				}
				for _, line := range lines[start:] {
					fmt.Printf("  %q\n", line) // Use %q to show escape sequences
				}
				failures++
			} else if strings.Contains(output, "PASS") || strings.Contains(output, "[no test files]") {
				fmt.Printf("✅ PASSED\n")
			} else {
				fmt.Printf("⚠️  UNKNOWN STATUS\n")
				// Print output with escapes visible
				start := len(lines) - 20
				if start < 0 {
					start = 0
				}
				for _, line := range lines[start:] {
					fmt.Printf("  %q\n", line)
				}
			}

			// Check termios state
			fmt.Printf("  BEFORE: %s\n", formatTermios(before))
			fmt.Printf("  AFTER:  %s\n", formatTermios(after))

			if !termiosEqual(before, after) {
				fmt.Printf("⚠️  TTY STATE CORRUPTION DETECTED!\n")
				stateCorruptions++
			}
		}
	}

	fmt.Printf("\n════════════════════════════════════════════════════════════════\n")
	fmt.Printf("SUMMARY: %d packages tested, %d failures, %d state corruptions\n",
		len(packages), failures, stateCorruptions)
	fmt.Printf("════════════════════════════════════════════════════════════════\n")

	if failures > 0 || stateCorruptions > 0 {
		os.Exit(1)
	}
}
