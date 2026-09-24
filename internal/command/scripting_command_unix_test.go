//go:build unix

package command

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// TestScriptingCommand_Execute_SigtermWithoutListenerRunningProgram covers the
// `osm script` path: signal delivery must be installed before the script runs,
// so a script blocked in tea.run() (WaitForProgram) is force-cancelled by an
// unlistened SIGTERM and reports Node's default status (143) promptly instead
// of ignoring the signal. Delivery installed after execution would never be
// reached, because the bubbletea binding defers SIGINT/SIGTERM to the engine.
func TestScriptingCommand_Execute_SigtermWithoutListenerRunningProgram(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns JS runtime and delivers real signals")
	}

	cmd := NewScriptingCommand(config.NewConfig())
	cmd.testMode = true
	cmd.store = "memory"
	cmd.session = t.Name()

	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "prog.js")
	content := `
		var tea = require("osm:bubbletea");
		function init() { return [{ n: 0 }, tea.tick(60000, "tick")]; }
		function update(msg, m) { return [m, tea.tick(60000, "tick")]; }
		function view(m) { return { content: "probe " + m.n }; }
		tea.run(tea.newModel({ init: init, update: update, view: view }));
	`
	if err := os.WriteFile(scriptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write script: %v", err)
	}

	go func() {
		time.Sleep(500 * time.Millisecond)
		if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
			t.Logf("Kill: %v", err)
		}
	}()

	var stdout, stderr bytes.Buffer
	start := time.Now()
	err := cmd.Execute([]string{scriptPath}, &stdout, &stderr)
	elapsed := time.Since(start)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 143 {
		t.Fatalf("Execute error = %v, want exit status 143 for unlistened SIGTERM with a running program\nstdout=%q\nstderr=%q", err, stdout.String(), stderr.String())
	}
	if elapsed > 5*time.Second {
		t.Fatalf("SIGTERM with a running program took %v; the program was not quit promptly", elapsed)
	}
}
