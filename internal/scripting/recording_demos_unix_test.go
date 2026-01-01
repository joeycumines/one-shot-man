//go:build unix

package scripting

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
)

// Recording integration tests that generate VHS tapes as a side effect.
// These tests ALWAYS run the application and verify correct behavior.
// Recording output (tapes and GIFs) is only generated when the appropriate flags are set.
//
// IMPORTANT: These tests use InputCaptureRecorder with the new options pattern.
// Commands are typed into a shell (not run directly) to ensure .tape files
// replay correctly with VHS.
//
// Usage:
//
//	go test -v -run TestRecording                      # Run tests only (no output files)
//	go test -v -run TestRecording -record              # Run tests + generate tapes
//	go test -v -run TestRecording -record -execute-vhs # Run tests + generate tapes + GIFs

// executeVHSOnTape runs VHS on a tape file to generate the GIF.
func executeVHSOnTape(ctx context.Context, tapePath string) error {
	vhsPath, err := exec.LookPath("vhs")
	if err != nil {
		return fmt.Errorf("vhs not found: %w", err)
	}
	cmd := exec.CommandContext(ctx, vhsPath, tapePath)
	cmd.Dir = filepath.Dir(tapePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("vhs execution failed: %w\nOutput: %s", err, output)
	}
	return nil
}

// ensurePrompt waits for a shell prompt or verifies it's already present.
// This guards against races by ensuring that any trailing '$' is the active
// shell prompt (it must be the last significant character on the line and
// not followed by other non-whitespace text). If not present, it waits for a
// real prompt to appear within the timeout.
func ensurePrompt(ctx context.Context, t *testing.T, r *InputCaptureRecorder, timeout time.Duration) {
	t.Helper()

	// 1. Capture state ONCE.
	snap := r.Snapshot()

	// 2. Define logic (unchanged)
	isPrompt := func(buf string) bool {
		idx := strings.LastIndex(buf, "$")
		if idx == -1 {
			return false
		}
		tail := buf[idx:]
		return strings.TrimRight(tail, " \t\r\n") == "$"
	}

	// 3. Check the captured state.
	// Note: Assuming r.Console().String() matches the snapshot state,
	// or better, if Snapshot has a String() method, use that.
	// If not, r.String() is acceptable ONLY if we accept that 'snap'
	// might be slightly older than 'r.String()', but never newer.
	// Ideally:
	currentBuf := r.String()
	if isPrompt(currentBuf) {
		return
	}

	// 4. Wait relative to the snapshot taken BEFORE the check.
	// Even if the prompt arrived during step 3, 'snap' (step 1)
	// does NOT contain it, so 'Expect' will see it as new content.
	promptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	condition := func(output string) bool {
		return isPrompt(output)
	}

	if err := r.Expect(promptCtx, snap, condition, "wait for strict prompt '$'"); err != nil {
		t.Fatalf("Expected prompt '$' at tail: %v\nBuffer: %q", err, r.String())
	}
}

// waitForProcessExit waits for the spawned shell to exit with code 0 and emits
// the recorder buffer in the error to aid debugging in case of timeout/failure.
// The timeout context is derived from the parent test context to ensure
// cancellation propagates.
func waitForProcessExit(ctx context.Context, t *testing.T, cp *termtest.Console, r *InputCaptureRecorder) {
	t.Helper()
	exitCtx, exitCancel := context.WithTimeout(ctx, 30*time.Second)
	defer exitCancel()
	if code, err := cp.WaitExit(exitCtx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)\nBuffer: %q", code, err, r.String())
	}
}

// buildRecorderOpts returns recorder options, including WithRecorderSkipTapeOutput()
// if recordingEnabled is false. This ensures tests always run but only write
// tape files when -record flag is set.
func buildRecorderOpts(baseOpts ...RecorderOption) []RecorderOption {
	if !recordingEnabled {
		return append(baseOpts, WithRecorderSkipTapeOutput())
	}
	return baseOpts
}

// typeString types a string character-by-character with a small delay.
func typeString(t *testing.T, recorder *InputCaptureRecorder, s string) {
	t.Helper()
	for _, ch := range s {
		if err := recorder.SendKey(string(ch)); err != nil {
			t.Fatalf("Failed to send key %q: %v", string(ch), err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// typeStringFast types a string with minimal delay.
func typeStringFast(t *testing.T, recorder *InputCaptureRecorder, s string) {
	t.Helper()
	for _, ch := range s {
		var key string
		if ch == '\n' {
			key = "\r"
		} else {
			key = string(ch)
		}
		if err := recorder.SendKey(key); err != nil {
			t.Fatalf("Failed to send key %q: %v", key, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestRecording_SuperDocument_Visual demonstrates the super-document visual TUI mode.
// This recording shows:
// - Starting the visual TUI
// - Adding documents using keyboard
// - Navigating between documents
// - Copying the prompt
func TestRecording_SuperDocument_Visual(t *testing.T) {
	outputDir := getRecordingOutputDir()
	tapePath := filepath.Join(outputDir, "super-document-visual.tape")
	gifPath := filepath.Join(outputDir, "super-document-visual.gif")

	// Create temp config to ensure isolation
	tempConfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(tempConfig, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	// Ensure process env is isolated and harmless: don't touch real clipboard, use memory store and a test-specific session
	t.Setenv("OSM_CONFIG", tempConfig)
	t.Setenv("OSM_CLIPBOARD", "cat > /dev/null")
	t.Setenv("OSM_STORE", "memory")
	t.Setenv("OSM_SESSION", "demo-super-doc-visual")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	settings := DefaultVHSRecordSettings()
	settings.OutputGIF = filepath.Base(gifPath)

	recorder, err := NewInputCaptureRecorder(ctx, tapePath, buildRecorderOpts(
		WithRecorderShell("bash"),
		WithRecorderCommand("osm", "super-document"),
		WithRecorderTimeout(30*time.Second),
		WithRecorderEnv(
			"OSM_SESSION=demo-super-doc-visual",
			"OSM_STORE=memory",
			"OSM_CLIPBOARD=cat > /dev/null",
			"OSM_CONFIG="+tempConfig,
			"TERM=xterm-256color",
		),
		WithRecorderVHSSettings(settings),
	)...)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Errorf("Failed to close recorder: %v", err)
		}
	}()

	cp := recorder.Console()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := recorder.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, recorder.String())
		}
	}

	// Wait for shell prompt
	snap := recorder.Snapshot()
	expect(snap, "$", 10*time.Second)
	recorder.RecordSleep(200 * time.Millisecond)

	// Type the command
	recorder.RecordComment("Run osm super-document")
	if err := recorder.TypeCommand(); err != nil {
		t.Fatalf("Failed to type command: %v", err)
	}
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	// Wait for TUI to render
	snap = recorder.Snapshot()
	expect(snap, "Super-Document Builder", 15*time.Second)
	expect(snap, "Documents: 0", 5*time.Second)
	recorder.RecordSleep(500 * time.Millisecond)

	// Add first document via keyboard
	recorder.RecordComment("Add first document")
	snap = recorder.Snapshot()
	if err := recorder.SendKey("a"); err != nil {
		t.Fatalf("Failed to send key: %v", err)
	}
	expect(snap, "Content (multi-line):", 5*time.Second)

	// Tab to content and type
	if err := recorder.SendKey("\t"); err != nil {
		t.Fatalf("Failed to send tab: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	content1 := "# Project Requirements\n\n- User authentication\n- Data persistence\n- API endpoints"
	typeStringFast(t, recorder, content1)

	// Tab to submit
	if err := recorder.SendKey("\t"); err != nil {
		t.Fatalf("Failed to send tab: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	snap = recorder.Snapshot()
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Documents: 1", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Add second document
	recorder.RecordComment("Add second document")
	snap = recorder.Snapshot()
	if err := recorder.SendKey("a"); err != nil {
		t.Fatalf("Failed to send key: %v", err)
	}
	expect(snap, "Content (multi-line):", 5*time.Second)

	if err := recorder.SendKey("\t"); err != nil {
		t.Fatalf("Failed to send tab: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	content2 := "# Technical Specs\n\nBackend: Go with Goja JS runtime\nFrontend: Terminal UI (Bubble Tea)"
	typeStringFast(t, recorder, content2)

	if err := recorder.SendKey("\t"); err != nil {
		t.Fatalf("Failed to send tab: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	snap = recorder.Snapshot()
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Documents: 2", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Navigate between documents
	recorder.RecordComment("Navigate between documents")
	if err := recorder.SendKey("\x1b[B"); err != nil { // Down
		t.Fatalf("Failed to send down: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	recorder.RecordSleep(300 * time.Millisecond)

	if err := recorder.SendKey("\x1b[A"); err != nil { // Up
		t.Fatalf("Failed to send up: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	recorder.RecordSleep(300 * time.Millisecond)

	// Copy prompt
	recorder.RecordComment("Copy the combined prompt")
	snap = recorder.Snapshot()
	if err := recorder.SendKey("c"); err != nil {
		t.Fatalf("Failed to send key: %v", err)
	}
	expect(snap, "Copied prompt", 5*time.Second)
	recorder.RecordSleep(600 * time.Millisecond)

	// Quit the application
	recorder.RecordComment("Quit")
	if err := recorder.SendKey("q"); err != nil {
		t.Fatalf("Failed to send key: %v", err)
	}
	recorder.RecordSleep(300 * time.Millisecond)

	// Wait for shell prompt to return
	ensurePrompt(ctx, t, recorder, 5*time.Second)

	// Exit the shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	waitForProcessExit(ctx, t, cp, recorder)

	if err := recorder.Close(); err != nil {
		t.Fatalf("Failed to save tape: %v", err)
	}

	if recordingEnabled {
		t.Logf("Recording tape saved to: %s", tapePath)
	}

	if executeVHSEnabled && recordingEnabled {
		if err := executeVHSOnTape(ctx, tapePath); err != nil {
			t.Fatalf("Failed to execute VHS: %v", err)
		}
		t.Logf("GIF generated at: %s", gifPath)
	}
}

// TestRecording_SuperDocument_Shell demonstrates the super-document shell mode.
func TestRecording_SuperDocument_Shell(t *testing.T) {
	outputDir := getRecordingOutputDir()
	tapePath := filepath.Join(outputDir, "super-document-shell.tape")
	gifPath := filepath.Join(outputDir, "super-document-shell.gif")

	// Create temp config to ensure isolation
	tempConfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(tempConfig, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	// Ensure process env is isolated
	t.Setenv("OSM_CONFIG", tempConfig)
	t.Setenv("OSM_CLIPBOARD", "cat > /dev/null")
	t.Setenv("OSM_STORE", "memory")
	t.Setenv("OSM_SESSION", "demo-super-doc-shell")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	settings := DefaultVHSRecordSettings()
	settings.OutputGIF = filepath.Base(gifPath)

	recorder, err := NewInputCaptureRecorder(ctx, tapePath, buildRecorderOpts(
		WithRecorderShell("bash"),
		WithRecorderCommand("osm", "super-document", "--shell"),
		WithRecorderTimeout(30*time.Second),
		WithRecorderEnv(
			"OSM_SESSION=demo-super-doc-shell",
			"OSM_STORE=memory",
			"OSM_CLIPBOARD=cat > /dev/null",
			"OSM_CONFIG="+tempConfig,
			"TERM=xterm-256color",
		),
		WithRecorderVHSSettings(settings),
	)...)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Errorf("Failed to close recorder: %v", err)
		}
	}()

	cp := recorder.Console()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := recorder.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, recorder.String())
		}
	}

	// Wait for shell prompt
	snap := recorder.Snapshot()
	expect(snap, "$", 10*time.Second)
	recorder.RecordSleep(200 * time.Millisecond)

	// Type the command
	recorder.RecordComment("Run osm super-document --shell")
	if err := recorder.TypeCommand(); err != nil {
		t.Fatalf("Failed to type command: %v", err)
	}
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	// Wait for super-document shell prompt
	snap = recorder.Snapshot()
	expect(snap, "(super-document)", 15*time.Second)
	recorder.RecordSleep(500 * time.Millisecond)

	// Show help
	recorder.RecordComment("Show available commands")
	snap = recorder.Snapshot()
	typeString(t, recorder, "help")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Available commands:", 5*time.Second)
	recorder.RecordSleep(800 * time.Millisecond)

	// Add a document
	recorder.RecordComment("Add a document")
	snap = recorder.Snapshot()
	typeString(t, recorder, "doc-add API Documentation for the project")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Added document", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// List documents
	recorder.RecordComment("List documents")
	snap = recorder.Snapshot()
	typeString(t, recorder, "list")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "API Documentation", 5*time.Second)
	recorder.RecordSleep(500 * time.Millisecond)

	// Show prompt
	recorder.RecordComment("Show the generated prompt")
	snap = recorder.Snapshot()
	typeString(t, recorder, "show")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "DOCUMENTS", 5*time.Second)
	time.Sleep(500 * time.Millisecond)
	recorder.RecordSleep(600 * time.Millisecond)

	// Copy
	recorder.RecordComment("Copy prompt to clipboard")
	snap = recorder.Snapshot()
	typeString(t, recorder, "copy")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Copied", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Exit the osm shell
	recorder.RecordComment("Exit osm shell")
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	recorder.RecordSleep(300 * time.Millisecond)

	// Wait for outer shell prompt
	ensurePrompt(ctx, t, recorder, 5*time.Second)

	// Exit the outer shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	waitForProcessExit(ctx, t, cp, recorder)

	if err := recorder.Close(); err != nil {
		t.Fatalf("Failed to save tape: %v", err)
	}

	if recordingEnabled {
		t.Logf("Recording tape saved to: %s", tapePath)
	}

	if executeVHSEnabled && recordingEnabled {
		if err := executeVHSOnTape(ctx, tapePath); err != nil {
			t.Fatalf("Failed to execute VHS: %v", err)
		}
		t.Logf("GIF generated at: %s", gifPath)
	}
}

// TestRecording_SuperDocument_Interop demonstrates visual<->shell mode switching.
func TestRecording_SuperDocument_Interop(t *testing.T) {
	outputDir := getRecordingOutputDir()
	tapePath := filepath.Join(outputDir, "super-document-interop.tape")
	gifPath := filepath.Join(outputDir, "super-document-interop.gif")

	// Create temp config to ensure isolation
	tempConfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(tempConfig, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	// Ensure process env is isolated
	t.Setenv("OSM_CONFIG", tempConfig)
	t.Setenv("OSM_CLIPBOARD", "cat > /dev/null")
	t.Setenv("OSM_STORE", "memory")
	t.Setenv("OSM_SESSION", "demo-super-doc-interop")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	settings := DefaultVHSRecordSettings()
	settings.OutputGIF = filepath.Base(gifPath)

	recorder, err := NewInputCaptureRecorder(ctx, tapePath, buildRecorderOpts(
		WithRecorderShell("bash"),
		WithRecorderCommand("osm", "super-document"),
		WithRecorderTimeout(30*time.Second),
		WithRecorderEnv(
			"OSM_SESSION=demo-super-doc-interop",
			"OSM_STORE=memory",
			"OSM_CLIPBOARD=cat > /dev/null",
			"OSM_CONFIG="+tempConfig,
			"TERM=xterm-256color",
		),
		WithRecorderVHSSettings(settings),
	)...)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Errorf("Failed to close recorder: %v", err)
		}
	}()

	cp := recorder.Console()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := recorder.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, recorder.String())
		}
	}

	// Wait for shell prompt
	snap := recorder.Snapshot()
	expect(snap, "$", 10*time.Second)
	recorder.RecordSleep(200 * time.Millisecond)

	// Type the command
	recorder.RecordComment("Run osm super-document")
	if err := recorder.TypeCommand(); err != nil {
		t.Fatalf("Failed to type command: %v", err)
	}
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	// Wait for TUI
	snap = recorder.Snapshot()
	expect(snap, "Super-Document Builder", 15*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Add document in visual mode
	recorder.RecordComment("Add document in VISUAL mode")
	snap = recorder.Snapshot()
	if err := recorder.SendKey("a"); err != nil {
		t.Fatalf("Failed to send key: %v", err)
	}
	expect(snap, "Content (multi-line):", 5*time.Second)

	if err := recorder.SendKey("\t"); err != nil {
		t.Fatalf("Failed to send tab: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	content := "Document created in VISUAL mode"
	typeStringFast(t, recorder, content)

	if err := recorder.SendKey("\t"); err != nil {
		t.Fatalf("Failed to send tab: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	snap = recorder.Snapshot()
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Documents: 1", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Switch to shell mode
	recorder.RecordComment("Switch to SHELL mode (press 's')")
	snap = recorder.Snapshot()
	if err := recorder.SendKey("s"); err != nil {
		t.Fatalf("Failed to send key: %v", err)
	}
	expect(snap, "(super-document)", 10*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Add document in shell mode
	recorder.RecordComment("Add document in SHELL mode")
	typeString(t, recorder, "doc-add Document added via SHELL mode")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	snap = recorder.Snapshot()
	expect(snap, "Added document", 5*time.Second)
	recorder.RecordSleep(300 * time.Millisecond)

	// List to show both
	recorder.RecordComment("List shows documents from both modes")
	snap = recorder.Snapshot()
	typeString(t, recorder, "list")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Document", 5*time.Second)
	recorder.RecordSleep(500 * time.Millisecond)

	// Switch back to TUI
	recorder.RecordComment("Switch back to VISUAL mode")
	snap = recorder.Snapshot()
	typeString(t, recorder, "tui")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Documents: 2", 10*time.Second)
	recorder.RecordSleep(600 * time.Millisecond)

	// Navigate
	if err := recorder.SendKey("\x1b[B"); err != nil { // Down
		t.Fatalf("Failed to send down: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	recorder.RecordSleep(300 * time.Millisecond)

	if err := recorder.SendKey("\x1b[A"); err != nil { // Up
		t.Fatalf("Failed to send up: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	recorder.RecordSleep(300 * time.Millisecond)

	// Quit the application
	if err := recorder.SendKey("q"); err != nil {
		t.Fatalf("Failed to send key: %v", err)
	}
	recorder.RecordSleep(300 * time.Millisecond)

	// Wait for shell prompt to return
	snap = recorder.Snapshot()
	expect(snap, "$", 5*time.Second)

	// Exit the shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	waitForProcessExit(ctx, t, cp, recorder)

	if err := recorder.Close(); err != nil {
		t.Fatalf("Failed to save tape: %v", err)
	}

	if recordingEnabled {
		t.Logf("Recording tape saved to: %s", tapePath)
	}

	if executeVHSEnabled && recordingEnabled {
		if err := executeVHSOnTape(ctx, tapePath); err != nil {
			t.Fatalf("Failed to execute VHS: %v", err)
		}
		t.Logf("GIF generated at: %s", gifPath)
	}
}

// TestRecording_CodeReview demonstrates the code-review command.
func TestRecording_CodeReview(t *testing.T) {
	outputDir := getRecordingOutputDir()
	tapePath := filepath.Join(outputDir, "code-review.tape")
	gifPath := filepath.Join(outputDir, "code-review.gif")

	// Create temp config to ensure isolation
	tempConfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(tempConfig, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	// Ensure process env is isolated
	t.Setenv("OSM_CONFIG", tempConfig)
	t.Setenv("OSM_CLIPBOARD", "cat > /dev/null")
	t.Setenv("OSM_STORE", "memory")
	t.Setenv("OSM_SESSION", "demo-code-review")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	settings := DefaultVHSRecordSettings()
	settings.OutputGIF = filepath.Base(gifPath)

	recorder, err := NewInputCaptureRecorder(ctx, tapePath, buildRecorderOpts(
		WithRecorderShell("bash"),
		WithRecorderCommand("osm", "code-review"),
		WithRecorderTimeout(30*time.Second),
		WithRecorderDir(outputDir),
		WithRecorderEnv(
			"OSM_SESSION=demo-code-review",
			"OSM_STORE=memory",
			"OSM_CLIPBOARD=cat > /dev/null",
			"OSM_CONFIG="+tempConfig,
			"TERM=xterm-256color",
		),
		WithRecorderVHSSettings(settings),
	)...)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Errorf("Failed to close recorder: %v", err)
		}
	}()

	cp := recorder.Console()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := recorder.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, recorder.String())
		}
	}

	// Wait for shell prompt
	snap := recorder.Snapshot()
	expect(snap, "$", 10*time.Second)
	recorder.RecordSleep(200 * time.Millisecond)

	// Type the command
	recorder.RecordComment("Run osm code-review")
	if err := recorder.TypeCommand(); err != nil {
		t.Fatalf("Failed to type command: %v", err)
	}
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	snap = recorder.Snapshot()
	expect(snap, "(code-review)", 15*time.Second)
	recorder.RecordSleep(500 * time.Millisecond)

	// Show help
	recorder.RecordComment("Show available commands")
	snap = recorder.Snapshot()
	typeString(t, recorder, "help")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Available commands:", 5*time.Second)
	recorder.RecordSleep(800 * time.Millisecond)

	// Add file context
	recorder.RecordComment("Add file to review context")
	snap = recorder.Snapshot()
	typeString(t, recorder, "add README.md")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Added file", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	recorder.RecordSleep(400 * time.Millisecond)

	// Add a note
	recorder.RecordComment("Add review focus note")
	snap = recorder.Snapshot()
	typeString(t, recorder, "note Focus on documentation clarity")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Added note", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	recorder.RecordSleep(400 * time.Millisecond)

	// List context
	recorder.RecordComment("List review context")
	snap = recorder.Snapshot()
	typeString(t, recorder, "list")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "[file]", 5*time.Second)
	recorder.RecordSleep(500 * time.Millisecond)

	// Show prompt
	recorder.RecordComment("Show the generated review prompt")
	snap = recorder.Snapshot()
	typeString(t, recorder, "show")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "GUARANTEE", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	recorder.RecordSleep(600 * time.Millisecond)

	// Copy
	recorder.RecordComment("Copy to clipboard")
	snap = recorder.Snapshot()
	typeString(t, recorder, "copy")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "copied", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Exit the osm shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	recorder.RecordSleep(300 * time.Millisecond)

	// Wait for outer shell prompt
	snap = recorder.Snapshot()
	expect(snap, "$", 5*time.Second)

	// Exit the outer shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	waitForProcessExit(ctx, t, cp, recorder)

	if err := recorder.Close(); err != nil {
		t.Fatalf("Failed to save tape: %v", err)
	}

	if recordingEnabled {
		t.Logf("Recording tape saved to: %s", tapePath)
	}

	if executeVHSEnabled && recordingEnabled {
		if err := executeVHSOnTape(ctx, tapePath); err != nil {
			t.Fatalf("Failed to execute VHS: %v", err)
		}
		t.Logf("GIF generated at: %s", gifPath)
	}
}

// TestRecording_PromptFlow demonstrates the prompt-flow command.
func TestRecording_PromptFlow(t *testing.T) {
	outputDir := getRecordingOutputDir()
	tapePath := filepath.Join(outputDir, "prompt-flow.tape")
	gifPath := filepath.Join(outputDir, "prompt-flow.gif")

	// Create temp config to ensure isolation
	tempConfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(tempConfig, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	// Ensure process env is isolated
	t.Setenv("OSM_CONFIG", tempConfig)
	t.Setenv("OSM_CLIPBOARD", "cat > /dev/null")
	t.Setenv("OSM_STORE", "memory")
	t.Setenv("OSM_SESSION", "demo-prompt-flow")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	settings := DefaultVHSRecordSettings()
	settings.OutputGIF = filepath.Base(gifPath)

	recorder, err := NewInputCaptureRecorder(ctx, tapePath, buildRecorderOpts(
		WithRecorderShell("bash"),
		WithRecorderCommand("osm", "prompt-flow"),
		WithRecorderTimeout(30*time.Second),
		WithRecorderEnv(
			"OSM_SESSION=demo-prompt-flow",
			"OSM_STORE=memory",
			"OSM_CLIPBOARD=cat > /dev/null",
			"OSM_CONFIG="+tempConfig,
			"TERM=xterm-256color",
		),
		WithRecorderVHSSettings(settings),
	)...)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Errorf("Failed to close recorder: %v", err)
		}
	}()

	cp := recorder.Console()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := recorder.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, recorder.String())
		}
	}

	// Wait for shell prompt
	snap := recorder.Snapshot()
	expect(snap, "$", 10*time.Second)
	recorder.RecordSleep(200 * time.Millisecond)

	// Type the command
	recorder.RecordComment("Run osm prompt-flow")
	if err := recorder.TypeCommand(); err != nil {
		t.Fatalf("Failed to type command: %v", err)
	}
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	snap = recorder.Snapshot()
	expect(snap, "(prompt-flow)", 15*time.Second)
	recorder.RecordSleep(500 * time.Millisecond)

	// Show help
	recorder.RecordComment("Show available commands")
	snap = recorder.Snapshot()
	typeString(t, recorder, "help")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Available commands:", 5*time.Second)
	recorder.RecordSleep(800 * time.Millisecond)

	// Set goal
	recorder.RecordComment("Set the development goal")
	snap = recorder.Snapshot()
	typeString(t, recorder, "goal Implement comprehensive test coverage")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Goal set.", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Generate meta-prompt
	recorder.RecordComment("Generate the meta-prompt")
	snap = recorder.Snapshot()
	typeString(t, recorder, "generate")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Meta-prompt generated", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Show meta prompt
	recorder.RecordComment("Show the meta-prompt for LLM")
	snap = recorder.Snapshot()
	typeString(t, recorder, "show meta")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "test coverage", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	recorder.RecordSleep(600 * time.Millisecond)

	// Set task
	recorder.RecordComment("Set task prompt (from LLM response)")
	snap = recorder.Snapshot()
	typeString(t, recorder, "use Write unit tests for authentication handlers")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Task prompt set", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Show final prompt
	recorder.RecordComment("Show the final generated prompt")
	snap = recorder.Snapshot()
	typeString(t, recorder, "show")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "prompt-flow", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	recorder.RecordSleep(600 * time.Millisecond)

	// Copy
	recorder.RecordComment("Copy to clipboard")
	snap = recorder.Snapshot()
	typeString(t, recorder, "copy")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "copied", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Exit the osm shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	recorder.RecordSleep(300 * time.Millisecond)

	// Wait for outer shell prompt
	snap = recorder.Snapshot()
	expect(snap, "$", 5*time.Second)

	// Exit the outer shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	waitForProcessExit(ctx, t, cp, recorder)

	if err := recorder.Close(); err != nil {
		t.Fatalf("Failed to save tape: %v", err)
	}

	if recordingEnabled {
		t.Logf("Recording tape saved to: %s", tapePath)
	}

	if executeVHSEnabled && recordingEnabled {
		if err := executeVHSOnTape(ctx, tapePath); err != nil {
			t.Fatalf("Failed to execute VHS: %v", err)
		}
		t.Logf("GIF generated at: %s", gifPath)
	}
}

// TestRecording_Goal demonstrates the goal command with a sample workflow.
func TestRecording_Goal(t *testing.T) {
	outputDir := getRecordingOutputDir()
	tapePath := filepath.Join(outputDir, "goal.tape")
	gifPath := filepath.Join(outputDir, "goal.gif")

	// Create temp config to ensure isolation
	tempConfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(tempConfig, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	// Ensure process env is isolated
	t.Setenv("OSM_CONFIG", tempConfig)
	t.Setenv("OSM_CLIPBOARD", "cat > /dev/null")
	t.Setenv("OSM_STORE", "memory")
	t.Setenv("OSM_SESSION", "demo-goal")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	settings := DefaultVHSRecordSettings()
	settings.OutputGIF = filepath.Base(gifPath)

	recorder, err := NewInputCaptureRecorder(ctx, tapePath, buildRecorderOpts(
		WithRecorderShell("bash"),
		WithRecorderCommand("osm", "goal", "test-generator"),
		WithRecorderTimeout(30*time.Second),
		WithRecorderEnv(
			"OSM_SESSION=demo-goal",
			"OSM_STORE=memory",
			"OSM_CLIPBOARD=cat > /dev/null",
			"OSM_CONFIG="+tempConfig,
			"TERM=xterm-256color",
		),
		WithRecorderVHSSettings(settings),
	)...)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Errorf("Failed to close recorder: %v", err)
		}
	}()

	cp := recorder.Console()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := recorder.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, recorder.String())
		}
	}

	// Wait for shell prompt
	snap := recorder.Snapshot()
	expect(snap, "$", 10*time.Second)
	recorder.RecordSleep(200 * time.Millisecond)

	// Type the command
	recorder.RecordComment("Run osm goal test-generator")
	if err := recorder.TypeCommand(); err != nil {
		t.Fatalf("Failed to type command: %v", err)
	}
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	snap = recorder.Snapshot()
	expect(snap, "test-gen", 15*time.Second)
	recorder.RecordSleep(500 * time.Millisecond)

	// Show help
	snap = recorder.Snapshot()
	typeString(t, recorder, "help")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Available commands:", 5*time.Second)
	recorder.RecordSleep(600 * time.Millisecond)

	// Show prompt
	snap = recorder.Snapshot()
	typeString(t, recorder, "show")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "test", 5*time.Second)
	time.Sleep(300 * time.Millisecond)
	recorder.RecordSleep(600 * time.Millisecond)

	// Copy
	snap = recorder.Snapshot()
	typeString(t, recorder, "copy")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "copied", 5*time.Second)
	recorder.RecordSleep(400 * time.Millisecond)

	// Exit the osm shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	recorder.RecordSleep(300 * time.Millisecond)

	// Wait for outer shell prompt
	snap = recorder.Snapshot()
	expect(snap, "$", 5*time.Second)

	// Exit the outer shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	waitForProcessExit(ctx, t, cp, recorder)

	if err := recorder.Close(); err != nil {
		t.Fatalf("Failed to save tape: %v", err)
	}

	if recordingEnabled {
		t.Logf("Recording tape saved to: %s", tapePath)
	}

	if executeVHSEnabled && recordingEnabled {
		if err := executeVHSOnTape(ctx, tapePath); err != nil {
			t.Fatalf("Failed to execute VHS: %v", err)
		}
		t.Logf("GIF generated at: %s", gifPath)
	}
}

// TestRecording_Quickstart is a quick overview demo.
func TestRecording_Quickstart(t *testing.T) {
	outputDir := getRecordingOutputDir()
	tapePath := filepath.Join(outputDir, "quickstart.tape")
	gifPath := filepath.Join(outputDir, "quickstart.gif")

	// Create temp config to ensure isolation
	tempConfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(tempConfig, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	// Ensure process env is isolated
	t.Setenv("OSM_CONFIG", tempConfig)
	t.Setenv("OSM_CLIPBOARD", "cat > /dev/null")
	t.Setenv("OSM_STORE", "memory")
	t.Setenv("OSM_SESSION", "demo-quickstart")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	settings := DefaultVHSRecordSettings()
	settings.OutputGIF = filepath.Base(gifPath)

	recorder, err := NewInputCaptureRecorder(ctx, tapePath, buildRecorderOpts(
		WithRecorderShell("bash"),
		WithRecorderCommand("osm", "help"),
		WithRecorderTimeout(10*time.Second),
		WithRecorderEnv(
			"OSM_SESSION=demo-quickstart",
			"OSM_STORE=memory",
			"OSM_CLIPBOARD=cat > /dev/null",
			"OSM_CONFIG="+tempConfig,
			"TERM=xterm-256color",
		),
		WithRecorderVHSSettings(settings),
	)...)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Errorf("Failed to close recorder: %v", err)
		}
	}()

	cp := recorder.Console()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := recorder.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, recorder.String())
		}
	}

	// Wait for shell prompt
	snap := recorder.Snapshot()
	expect(snap, "$", 10*time.Second)
	recorder.RecordSleep(200 * time.Millisecond)

	// Type the command
	recorder.RecordComment("Show osm help")
	if err := recorder.TypeCommand(); err != nil {
		t.Fatalf("Failed to type command: %v", err)
	}
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	// Wait for help output to appear
	time.Sleep(2 * time.Second)
	recorder.RecordSleep(1 * time.Second)

	// Wait for shell prompt to return (help exits immediately)
	ensurePrompt(ctx, t, recorder, 5*time.Second)

	// Exit the shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	waitForProcessExit(ctx, t, cp, recorder)

	if err := recorder.Close(); err != nil {
		t.Fatalf("Failed to save tape: %v", err)
	}

	if recordingEnabled {
		t.Logf("Recording tape saved to: %s", tapePath)
	}

	if executeVHSEnabled && recordingEnabled {
		if err := executeVHSOnTape(ctx, tapePath); err != nil {
			t.Fatalf("Failed to execute VHS: %v", err)
		}
		t.Logf("GIF generated at: %s", gifPath)
	}
}

// TestRecording_SuperDocument_Visual_Light demonstrates light theme.
func TestRecording_SuperDocument_Visual_Light(t *testing.T) {
	outputDir := getRecordingOutputDir()
	tapePath := filepath.Join(outputDir, "super-document-visual-light.tape")
	gifPath := filepath.Join(outputDir, "super-document-visual-light.gif")

	// Create temp config to ensure isolation
	tempConfig := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(tempConfig, []byte{}, 0644); err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	// Ensure process env is isolated
	t.Setenv("OSM_CONFIG", tempConfig)
	t.Setenv("OSM_CLIPBOARD", "cat > /dev/null")
	t.Setenv("OSM_STORE", "memory")
	t.Setenv("OSM_SESSION", "demo-super-doc-light")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	settings := DefaultVHSRecordSettings()
	settings.OutputGIF = filepath.Base(gifPath)
	settings.Theme = VHSLightTheme
	settings.MarginFill = "#f8f8f2"

	recorder, err := NewInputCaptureRecorder(ctx, tapePath, buildRecorderOpts(
		WithRecorderShell("bash"),
		WithRecorderCommand("osm", "super-document"),
		WithRecorderTimeout(30*time.Second),
		WithRecorderEnv(
			"OSM_SESSION=demo-super-doc-light",
			"OSM_STORE=memory",
			"OSM_CLIPBOARD=cat > /dev/null",
			"OSM_CONFIG="+tempConfig,
			"TERM=xterm-256color",
		),
		WithRecorderVHSSettings(settings),
	)...)
	if err != nil {
		t.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Errorf("Failed to close recorder: %v", err)
		}
	}()

	cp := recorder.Console()

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := recorder.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, recorder.String())
		}
	}

	// Wait for shell prompt
	snap := recorder.Snapshot()
	expect(snap, "$", 10*time.Second)
	recorder.RecordSleep(200 * time.Millisecond)

	// Type the command
	recorder.RecordComment("Light theme demonstration")
	if err := recorder.TypeCommand(); err != nil {
		t.Fatalf("Failed to type command: %v", err)
	}
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	snap = recorder.Snapshot()
	expect(snap, "Super-Document Builder", 15*time.Second)
	recorder.RecordSleep(500 * time.Millisecond)

	// Add a document
	snap = recorder.Snapshot()
	if err := recorder.SendKey("a"); err != nil {
		t.Fatalf("Failed to send key: %v", err)
	}
	expect(snap, "Content (multi-line):", 5*time.Second)

	if err := recorder.SendKey("\t"); err != nil {
		t.Fatalf("Failed to send tab: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	content := "# Light Theme Demo\n\nThis demonstrates the light theme variant."
	typeStringFast(t, recorder, content)

	if err := recorder.SendKey("\t"); err != nil {
		t.Fatalf("Failed to send tab: %v", err)
	}
	time.Sleep(50 * time.Millisecond)

	snap = recorder.Snapshot()
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}
	expect(snap, "Documents: 1", 5*time.Second)
	recorder.RecordSleep(600 * time.Millisecond)

	// Copy
	if err := recorder.SendKey("c"); err != nil {
		t.Fatalf("Failed to send key: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	recorder.RecordSleep(400 * time.Millisecond)

	// Quit the application
	if err := recorder.SendKey("q"); err != nil {
		t.Fatalf("Failed to send key: %v", err)
	}
	recorder.RecordSleep(300 * time.Millisecond)

	// Wait for shell prompt to return
	snap = recorder.Snapshot()
	expect(snap, "$", 5*time.Second)

	// Exit the shell
	typeString(t, recorder, "exit")
	if err := recorder.SendKey("\r"); err != nil {
		t.Fatalf("Failed to send enter: %v", err)
	}

	waitForProcessExit(ctx, t, cp, recorder)

	if err := recorder.Close(); err != nil {
		t.Fatalf("Failed to save tape: %v", err)
	}

	if recordingEnabled {
		t.Logf("Recording tape saved to: %s", tapePath)
	}

	if executeVHSEnabled && recordingEnabled {
		if err := executeVHSOnTape(ctx, tapePath); err != nil {
			t.Fatalf("Failed to execute VHS: %v", err)
		}
		t.Logf("GIF generated at: %s", gifPath)
	}
}
