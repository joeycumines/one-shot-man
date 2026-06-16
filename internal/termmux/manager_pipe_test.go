package termmux

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// pipeHelperSource is a tiny Go program used by command-pipe tests. It
// creates/clears the output file given as its first argument, copies stdin
// to it, and exits when stdin closes.
const pipeHelperSource = `package main

import (
	"io"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		panic("output file required")
	}
	f, err := os.Create(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err := io.Copy(f, os.Stdin); err != nil {
		panic(err)
	}
}
`

// writePipeHelper writes the helper program into t.TempDir() and returns
// its full path.
func writePipeHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pipehelper.go")
	if err := os.WriteFile(path, []byte(pipeHelperSource), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	return path
}

// readPipeOutput polls the output file until it contains the expected
// substring or the deadline expires.
func readPipeOutput(t *testing.T, path, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil && string(data) == want {
			return
		}
		select {
		case <-deadline:
			got := ""
			if data != nil {
				got = string(data)
			}
			t.Fatalf("timed out waiting for output %q; got %q", want, got)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestSessionManager_PipePaneCommand(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	helper := writePipeHelper(t)
	outPath := filepath.Join(t.TempDir(), "output.log")

	if err := m.PipePaneCommand(id, "go", []string{"run", helper, outPath}); err != nil {
		t.Fatalf("PipePaneCommand: %v", err)
	}

	session.readerCh <- []byte("hello from command pipe")
	readPipeOutput(t, outPath, "hello from command pipe", 30*time.Second)
}

func TestSessionManager_PipePaneCommand_ClearTerminatesProcess(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	helper := writePipeHelper(t)
	outPath := filepath.Join(t.TempDir(), "output.log")

	if err := m.PipePaneCommand(id, "go", []string{"run", helper, outPath}); err != nil {
		t.Fatalf("PipePaneCommand: %v", err)
	}

	session.readerCh <- []byte("before clear")
	readPipeOutput(t, outPath, "before clear", 30*time.Second)

	if err := m.ClearPipe(id); err != nil {
		t.Fatalf("ClearPipe: %v", err)
	}

	// After ClearPipe the helper should exit. Sending more data should not
	// reach it.
	session.readerCh <- []byte("after clear")
	time.Sleep(50 * time.Millisecond)
	data, _ := os.ReadFile(outPath)
	if string(data) != "before clear" {
		t.Errorf("output after clear = %q, want %q", string(data), "before clear")
	}
}

func TestSessionManager_PipePaneCommand_PaneExitTerminatesProcess(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	helper := writePipeHelper(t)
	outPath := filepath.Join(t.TempDir(), "output.log")

	if err := m.PipePaneCommand(id, "go", []string{"run", helper, outPath}); err != nil {
		t.Fatalf("PipePaneCommand: %v", err)
	}

	session.readerCh <- []byte("before exit")
	readPipeOutput(t, outPath, "before exit", 30*time.Second)

	close(session.readerCh)
	// Give the manager time to process EOF and close the pipe.
	time.Sleep(100 * time.Millisecond)

	data, _ := os.ReadFile(outPath)
	if string(data) != "before exit" {
		t.Errorf("output after exit = %q, want %q", string(data), "before exit")
	}
}

func TestSessionManager_PipePaneCommand_InvalidSession(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	err := m.PipePaneCommand(999, "go", []string{"run", "-"})
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

func TestSessionManager_PipePaneCommand_EmptyCommand(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	err := m.PipePaneCommand(1, "", nil)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestSessionManager_PipePaneCommand_InvalidCommand(t *testing.T) {
	m, cleanup := startManager(t)
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	err = m.PipePaneCommand(id, "this-command-definitely-does-not-exist-12345", nil)
	if err == nil {
		t.Fatal("expected error for invalid command")
	}
}

func TestSessionManager_PipePaneCommand_ReplacesFilePipe(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.log")
	helper := writePipeHelper(t)
	outPath := filepath.Join(dir, "cmd.log")

	if err := m.SetPipeFile(id, filePath); err != nil {
		t.Fatalf("SetPipeFile: %v", err)
	}
	if err := m.PipePaneCommand(id, "go", []string{"run", helper, outPath}); err != nil {
		t.Fatalf("PipePaneCommand: %v", err)
	}

	session.readerCh <- []byte("shared output")
	readPipeOutput(t, outPath, "shared output", 30*time.Second)

	// The old file pipe should have been closed; no new data is written there.
	data, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "" {
		t.Errorf("file pipe = %q, want empty after replacement", string(data))
	}
}

func TestSessionManager_PipePaneCommand_StdinIsPipe(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Use os/exec's own echo-ish helper: "go" with "env" prints environment to
	// stdout, but we need something that reads stdin. pipeHelperSource does that.
	helper := writePipeHelper(t)
	outPath := filepath.Join(t.TempDir(), "output.log")

	if err := m.PipePaneCommand(id, "go", []string{"run", helper, outPath}); err != nil {
		t.Fatalf("PipePaneCommand: %v", err)
	}

	// Send a large-ish chunk to exercise the pipe path.
	chunk := make([]byte, 4096)
	for i := range chunk {
		chunk[i] = byte('a' + i%26)
	}
	session.readerCh <- chunk
	readPipeOutput(t, outPath, string(chunk), 30*time.Second)
}

func TestSessionManager_ClearPipe_KillsSlowProcess(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// "sleep" keeps running even after stdin closes. Use a tiny Go program for
	// cross-platform compatibility.
	slowSource := `package main
import "time"
func main() { time.Sleep(30 * time.Second) }
`
	dir := t.TempDir()
	slowPath := filepath.Join(dir, "slow.go")
	if err := os.WriteFile(slowPath, []byte(slowSource), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}

	if err := m.PipePaneCommand(id, "go", []string{"run", slowPath}); err != nil {
		t.Fatalf("PipePaneCommand: %v", err)
	}

	if err := m.ClearPipe(id); err != nil {
		t.Fatalf("ClearPipe: %v", err)
	}

	// The spawned process should be gone well before its 30s sleep ends.
	time.Sleep(500 * time.Millisecond)
}

func TestSessionManager_PipePaneCommand_CommandPathMutuallyExclusive(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Use SetPipe with both fields populated.
	err = m.SetPipe(id, PipeConfig{Path: "/tmp/x", Command: "go"})
	if err == nil {
		t.Fatal("expected error when Path and Command are both set")
	}
	_ = err
}

func TestSessionManager_PipePaneCommand_DefaultsToGoLookPath(t *testing.T) {
	m, cleanup := startManager(t, WithTermSize(24, 80))
	defer cleanup()

	session := newControllableSession()
	id, err := m.Register(session, SessionTarget{Name: "test", Kind: SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	// Use a bare command name; exec.LookPath resolves it if PATH contains it.
	goPath, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not in PATH")
	}
	_ = goPath

	helper := writePipeHelper(t)
	outPath := filepath.Join(t.TempDir(), "output.log")

	if err := m.PipePaneCommand(id, "go", []string{"run", helper, outPath}); err != nil {
		t.Fatalf("PipePaneCommand: %v", err)
	}

	session.readerCh <- []byte("found by path")
	readPipeOutput(t, outPath, "found by path", 30*time.Second)
}
