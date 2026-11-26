package pty

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

type argumentsOutput struct {
	data []byte
	err  error
}

func TestSpawn_WindowsVerbatimShell(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a native ConPTY command")
	}
	if runtime.GOOS != "windows" {
		t.Skip("requires the native Windows command interpreter")
	}
	requireConPTYRuntime(t)
	script := filepath.Join(t.TempDir(), "script with spaces.cmd")
	if err := os.WriteFile(script, []byte("@echo pty-verbatim-marker %~1\r\n@exit /b 0\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := `"` + script + `" "two words" && echo pty-operator-ok & exit /b 7`
	ctx, cancel := context.WithTimeout(context.Background(), conPTYOutputDeadline)
	defer cancel()
	proc, err := Spawn(ctx, SpawnConfig{
		Command:                  "cmd.exe",
		Args:                     []string{"/d", "/s", "/c", `"` + command + `"`},
		WindowsVerbatimArguments: true,
		Rows:                     24,
		Cols:                     120,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()
	output := make(chan argumentsOutput, 1)
	file := proc.File()
	go func() {
		data, err := io.ReadAll(file)
		output <- argumentsOutput{data: data, err: err}
	}()
	code, err := proc.Wait()
	if err != nil || code != 7 {
		t.Fatalf("shell exit=(%d,%v), want (7,nil)", code, err)
	}
	// The process has exited, but ConPTY owns the pipe writer until this
	// flush. Keep reading through its EOF before releasing the reader.
	proc.ClosePseudoConsole()
	var captured argumentsOutput
	select {
	case captured = <-output:
	case <-ctx.Done():
		t.Fatal("ConPTY did not finish flushing output")
	}
	if err := proc.Close(); err != nil {
		t.Fatal(err)
	}
	if captured.err != nil {
		t.Fatalf("ConPTY output read: %v", captured.err)
	}
	for _, text := range []string{"pty-verbatim-marker two words", "pty-operator-ok"} {
		if !strings.Contains(string(captured.data), text) {
			t.Errorf("ConPTY output lacks %q: %q", text, captured.data)
		}
	}
}

func TestSpawn_UnixIgnoresWindowsVerbatimArguments(t *testing.T) {
	if testing.Short() {
		t.Skip("runs native PTY commands")
	}
	if runtime.GOOS == "windows" {
		t.Skip("checks Unix argv semantics")
	}
	for _, verbatim := range []bool{false, true} {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		proc, err := Spawn(ctx, SpawnConfig{
			Command:                  "sh",
			Args:                     []string{"-c", `printf '<%s>\n' "$1" "$2"`, "arguments-test", "two words", `literal"&|()`},
			WindowsVerbatimArguments: verbatim,
			Rows:                     24,
			Cols:                     120,
		})
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		// Read concurrently: a PTY waiter must never depend on an undrained
		// output buffer. EIO after the last Unix PTY byte is an ordinary EOF.
		output := make(chan argumentsOutput, 1)
		file := proc.File()
		go func() {
			data, err := io.ReadAll(file)
			output <- argumentsOutput{data: data, err: err}
		}()
		code, waitErr := proc.Wait()
		var captured argumentsOutput
		select {
		case captured = <-output:
		case <-ctx.Done():
			_ = proc.Close()
			cancel()
			t.Fatal("PTY output did not reach EOF")
		}
		closeErr := proc.Close()
		cancel()
		if waitErr != nil || code != 0 || closeErr != nil {
			t.Fatalf("verbatim=%v: exit=%d wait=%v close=%v", verbatim, code, waitErr, closeErr)
		}
		if captured.err != nil && !errors.Is(captured.err, syscall.EIO) {
			t.Fatalf("PTY output read: %v", captured.err)
		}
		want := "<two words>\r\n<literal\"&|()>\r\n"
		if string(captured.data) != want {
			t.Errorf("verbatim=%v output=%q, want %q", verbatim, captured.data, want)
		}
	}
}
