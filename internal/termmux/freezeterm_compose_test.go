package termmux_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/freezeterm"
	"github.com/joeycumines/one-shot-man/internal/termmux"
)

// TestComposeCaptureWithFreezeterm is the single neutral integration test that
// composes the termmux capture surface with the independent freezeterm client.
// Neither production package imports the other; only this consumer-side test
// does.
func TestComposeCaptureWithFreezeterm(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: PTY session plus subprocess render")
	}
	stub := buildStub(t)
	marker := "compose-capture-marker"
	cmd, args := echoCommand(marker)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	manager := termmux.NewSessionManager(termmux.WithTermSize(10, 60))
	runErr := make(chan error, 1)
	go func() { runErr <- manager.Run(ctx) }()
	<-manager.Started()
	manager.SetRemainOnExit(true)

	session := termmux.NewCaptureSession(termmux.CaptureConfig{Command: cmd, Args: args, Rows: 10, Cols: 60})
	id, err := manager.Register(session, termmux.SessionTarget{Name: "compose", Kind: termmux.SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := session.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := manager.Activate(id); err != nil {
		t.Fatalf("Activate: %v", err)
	}
	defer func() {
		_ = session.Close()
		manager.Close()
		<-runErr
	}()

	var capture *termmux.Capture
	var captureErr error
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		capture, captureErr = manager.CaptureScreen(id, termmux.CaptureOptions{Kind: termmux.CaptureANSI})
		if captureErr == nil && strings.Contains(capture.Text, marker) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if capture == nil || !strings.Contains(capture.Text, marker) {
		t.Fatalf("capture did not observe %q: capture=%+v err=%v", marker, capture, captureErr)
	}

	svg, err := freezeterm.RenderText(ctx, freezeterm.Options{
		Executable: stub,
		Input:      []byte(capture.Text),
	})
	if err != nil {
		t.Fatalf("RenderText: %v", err)
	}
	if !strings.Contains(svg, "<svg") {
		t.Fatalf("render output is not SVG: %.100q", svg)
	}

	ranged, err := manager.CaptureScreen(id, termmux.CaptureOptions{Kind: termmux.CaptureANSI, Start: 0, End: 1})
	if err != nil {
		t.Fatalf("ranged CaptureScreen: %v", err)
	}
	if _, err := freezeterm.RenderText(ctx, freezeterm.Options{Executable: stub, Input: []byte(ranged.Text)}); err != nil {
		t.Fatalf("ranged RenderText: %v", err)
	}
}

// buildStub compiles the freeze stub owned by internal/freezeterm and returns
// its path.
func buildStub(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	exe := filepath.Join(dir, "freezestub")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-o", exe, filepath.Join("..", "freezeterm", "testdata", "stub"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub: %v: %s", err, out)
	}
	return exe
}

// echoCommand returns a platform-appropriate command that prints marker and
// stays alive briefly, so the manager's reader attaches before the child exits
// and the published snapshot can be captured deterministically.
func echoCommand(marker string) (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/c", "echo " + marker + " & ping -n 2 127.0.0.1 >nul"}
	}
	return "/bin/sh", []string{"-c", "printf '" + marker + "\\n'; sleep 1"}
}
