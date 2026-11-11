//go:build unix

package termpane

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	termmux "github.com/joeycumines/one-shot-man/internal/termmux"
)

// TestUpdate_AltLeftWireBytes proves the end-to-end key path: a BubbleTea
// alt+left keypress (the "option+left" word-skip combo on macOS) is encoded
// by termmux.KeyToTermBytes and arrives at the child PTY as exactly the xterm
// sequence ESC[1;3D — never as a bare 'b' (the ESC-prefix legacy encoding)
// and never split or altered.
//
// The child (buildHexEchoProgram) puts its own terminal into raw mode
// (ICANON|ECHO off) so the kernel line discipline neither echoes nor
// line-buffers the input, then writes every byte it reads to stdout as two
// lowercase hex digits. The test waits for the child's READY sentinel before
// sending input (avoiding a race with the child's termios setup), then asserts
// the captured hex equals exactly "1b5b313b3344" and does not contain the
// legacy "1b62" (ESC b) encoding.
func TestUpdate_AltLeftWireBytes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	bin := buildHexEchoProgram(t)

	cs := termmux.NewCaptureSession(termmux.CaptureConfig{
		Command: bin,
		Rows:    pasteBounds.Size.Height,
		Cols:    pasteBounds.Size.Width,
	})
	ctx := t.Context()
	if err := cs.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	mgr := termmux.NewSessionManager(termmux.WithTermSize(pasteBounds.Size.Height, pasteBounds.Size.Width))
	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(mgrCtx) }()
	<-mgr.Started()
	defer func() {
		mgrCancel()
		<-errCh
	}()

	sid, err := mgr.Register(cs, termmux.SessionTarget{Name: "wire", Kind: termmux.SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	model := NewModel(sid, mgr, pasteBounds)
	defer model.Close()

	// Wait for the child to report it has entered raw mode before sending
	// input, so the kernel line discipline (canonical+echo) cannot mangle the
	// bytes on the way in.
	readyDeadline := time.After(5 * time.Second)
	for {
		snap := mgr.Snapshot(sid)
		if snap != nil && strings.Contains(snapshotPlainText(snap), "READY") {
			break
		}
		select {
		case <-readyDeadline:
			snap := mgr.Snapshot(sid)
			text := ""
			if snap != nil {
				text = snapshotPlainText(snap)
			}
			t.Fatalf("timed out waiting for child READY sentinel; capture:\n%q", text)
		case <-time.After(10 * time.Millisecond):
		}
	}

	// alt+left as BubbleTea v2 decodes it: Code=KeyLeft, Mod=ModAlt.
	altLeft := tea.Key{Code: tea.KeyLeft, Mod: tea.ModAlt}
	if got := altLeft.String(); got != "alt+left" {
		t.Fatalf("sanity: alt+left key string = %q", got)
	}
	model.Update(tea.KeyPressMsg(altLeft))

	// The child hex-encodes every byte it reads. Wait for "1b5b313b3344"
	// (ESC [ 1 ; 3 D) to appear in the captured output, then assert it is the
	// ONLY input hex present (exact match) and that the legacy ESC b
	// encoding ("1b62") was not emitted.
	wantHex := "1b5b313b3344"
	deadline := time.After(5 * time.Second)
	for {
		snap := mgr.Snapshot(sid)
		if snap != nil {
			text := snapshotPlainText(snap)
			if found := strings.Contains(text, wantHex); found {
				// Strip the READY sentinel line, then the remainder must be
				// exactly the hex of ESC[1;3D (plus whitespace).
				rest := strings.TrimSpace(strings.Replace(text, "READY", "", 1))
				if rest != wantHex {
					t.Fatalf("child received unexpected input bytes: got hex %q, want exactly %q (legacy ESC b would appear as 1b62)", rest, wantHex)
				}
				if strings.Contains(rest, "1b62") {
					t.Fatalf("legacy ESC+b encoding leaked: %q", rest)
				}
				return
			}
		}
		select {
		case <-deadline:
			snap := mgr.Snapshot(sid)
			text := ""
			if snap != nil {
				text = snapshotPlainText(snap)
			}
			t.Fatalf("timed out waiting for hex %q (ESC[1;3D) in child output; capture:\n%q", wantHex, text)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// TestUpdate_AltLeftWireBytes_AppCursor verifies the SAME alt+left wire bytes
// when the child has enabled DECCKM (application cursor mode, DECSET ?1h) —
// the mode vim/zsh/neovim/BubbleTea children run in. Modified cursor keys
// must still arrive as ESC[1;3D (CSI), never as an SS3+modifier form that a
// VT parser would consume as an unknown SS3 plus a literal 'D'.
func TestUpdate_AltLeftWireBytes_AppCursor(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in -short mode")
	}

	bin := buildHexEchoProgram(t)

	cs := termmux.NewCaptureSession(termmux.CaptureConfig{
		Command: bin,
		Rows:    pasteBounds.Size.Height,
		Cols:    pasteBounds.Size.Width,
	})
	ctx := t.Context()
	if err := cs.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer cs.Close()

	mgr := termmux.NewSessionManager(termmux.WithTermSize(pasteBounds.Size.Height, pasteBounds.Size.Width))
	mgrCtx, mgrCancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(mgrCtx) }()
	<-mgr.Started()
	defer func() {
		mgrCancel()
		<-errCh
	}()

	sid, err := mgr.Register(cs, termmux.SessionTarget{Name: "wire-appcursor", Kind: termmux.SessionKindPTY})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	model := NewModel(sid, mgr, pasteBounds)
	defer model.Close()

	// Wait for READY, then flip the child into application-cursor mode by
	// injecting DECSET ?1h as child output (the manager's VTerm parses it and
	// the snapshot's ApplicationCursor becomes true, which termpane mirrors
	// into m.appCursor). This is the mode vim/zsh/neovim run in.
	readyDeadline := time.After(5 * time.Second)
	for {
		snap := mgr.Snapshot(sid)
		if snap != nil && strings.Contains(snapshotPlainText(snap), "READY") {
			break
		}
		select {
		case <-readyDeadline:
			t.Fatal("timed out waiting for child READY")
		case <-time.After(10 * time.Millisecond):
		}
	}

	// Inject DECSET ?1h as child output and wait for the snapshot (and thus
	// the model's appCursor mirror) to observe application-cursor mode.
	cs.InjectOutput([]byte("\x1b[?1h"))
	appDeadline := time.After(3 * time.Second)
	for {
		snap := mgr.Snapshot(sid)
		if snap != nil && snap.ApplicationCursor {
			break
		}
		select {
		case <-appDeadline:
			t.Fatal("timed out waiting for ApplicationCursor=true in snapshot")
		case <-time.After(10 * time.Millisecond):
		}
	}
	model.RefreshSnapshot()

	altLeft := tea.Key{Code: tea.KeyLeft, Mod: tea.ModAlt}
	model.Update(tea.KeyPressMsg(altLeft))

	wantHex := "1b5b313b3344"
	deadline := time.After(5 * time.Second)
	for {
		snap := mgr.Snapshot(sid)
		if snap != nil {
			text := snapshotPlainText(snap)
			if idx := strings.Index(text, wantHex); idx >= 0 {
				rest := strings.TrimSpace(strings.Replace(text, "READY", "", 1))
				if rest != wantHex {
					t.Fatalf("appCursor child received unexpected bytes: got %q, want exactly %q", rest, wantHex)
				}
				return
			}
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for ESC[1;3D hex on real PTY")
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// buildHexEchoProgram builds a small Unix helper that puts its stdin into
// raw mode (no echo, no canonical processing) and writes every byte it
// reads to stdout as two lowercase hex digits. This lets the test observe the
// exact bytes delivered to the child PTY.
func buildHexEchoProgram(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	src := dir + "/main.go"
	prog := `package main

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

func main() {
	fd := int(os.Stdin.Fd())
	old, err := term.MakeRaw(fd)
	if err != nil {
		// Not a terminal (shouldn't happen under a PTY) — continue anyway.
		_ = old
	} else {
		defer term.Restore(fd, old)
	}
	// Readiness sentinel: printed AFTER raw mode is active.
	fmt.Println("READY")
	os.Stdout.Sync()
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			for i := 0; i < n; i++ {
				fmt.Printf("%02x", buf[i])
			}
			// No per-chunk newline: callers hex-compare the echo and a
			// chunk-boundary 0x0a would be indistinguishable from payload.
			os.Stdout.Sync()
		}
		if err != nil {
			return
		}
	}
}
`
	if err := os.WriteFile(src, []byte(prog), 0o644); err != nil {
		t.Fatalf("write helper source: %v", err)
	}
	out := filepath.Join(dir, "hexecho")
	cmd := exec.Command("go", "build", "-o", out, src)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go build hexecho: %v\n%s", err, stderr.String())
	}
	return out
}
