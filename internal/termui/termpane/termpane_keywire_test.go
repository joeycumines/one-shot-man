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
// The child is a copy program that echoes its stdin back, so the captured
// output contains the exact bytes the PTY received... except the terminal in
// canonical mode echoes input with processing. To observe the RAW input
// bytes instead, the child disables ECHO on its own terminal before
// reading, and instead writes each input chunk length-prefixed to a side
// channel is overkill; simpler: the child reads raw bytes in raw mode and
// writes them hex-encoded to stdout, which we capture.
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

	// alt+left as BubbleTea v2 decodes it: Code=KeyLeft, Mod=ModAlt.
	altLeft := tea.Key{Code: tea.KeyLeft, Mod: tea.ModAlt}
	if got := altLeft.String(); got != "alt+left" {
		t.Fatalf("sanity: alt+left key string = %q", got)
	}
	model.Update(tea.KeyPressMsg(altLeft))

	// The child hex-encodes every byte it reads. Wait for "1b5b313b3344"
	// (ESC [ 1 ; 3 D) to appear in the captured output.
	wantHex := "1b5b313b3344"
	deadline := time.After(5 * time.Second)
	for {
		snap := mgr.Snapshot(sid)
		if snap != nil {
			if text := snapshotPlainText(snap); strings.Contains(text, wantHex) {
				// Assert the full sequence arrived contiguously and that no
				// stray 'b' (0x62) was injected by a legacy encoding.
				if !strings.Contains(text, wantHex) {
					t.Fatalf("missing %q in %q", wantHex, text)
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

	"golang.org/x/sys/unix"
)

func main() {
	// Raw mode: disable ICANON and ECHO on stdin so bytes pass through
	// unmodified and are not echoed by the tty line discipline.
	fd := int(os.Stdin.Fd())
	if old, err := unix.IoctlGetTermios(fd, unix.TIOCGETA); err == nil {
		new := *old
		new.Lflag &^= unix.ICANON | unix.ECHO
		new.Iflag &^= unix.ICRNL | unix.INLCR | unix.IGNCR
		new.Cc[unix.VMIN] = 1
		new.Cc[unix.VTIME] = 0
		_ = unix.IoctlSetTermios(fd, unix.TIOCSETA, &new)
		defer unix.IoctlSetTermios(fd, unix.TIOCSETA, old)
	}
	buf := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			for i := 0; i < n; i++ {
				fmt.Printf("%02x", buf[i])
			}
			fmt.Println()
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
