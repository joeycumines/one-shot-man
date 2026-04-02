package termmux

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestSessionTarget_String(t *testing.T) {
	tests := []struct {
		name string
		got  SessionTarget
		want string
	}{
		{name: "zero", got: SessionTarget{}, want: "unknown"},
		{name: "kind", got: SessionTarget{Kind: SessionKindPTY}, want: "pty"},
		{name: "name", got: SessionTarget{Name: "claude"}, want: "claude"},
		{name: "name-kind", got: SessionTarget{Name: "verify", Kind: SessionKindCapture}, want: "verify[capture]"},
		{name: "name-kind-id", got: SessionTarget{Name: "shell", Kind: SessionKindCapture, ID: "s-1"}, want: "shell[capture:s-1]"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.got.String(); got != tc.want {
				t.Fatalf("SessionTarget.String() = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestCaptureSession_Target(t *testing.T) {
	cs := NewCaptureSession(CaptureConfig{
		Name:    "verify",
		Kind:    SessionKindCapture,
		Command: "echo",
		Args:    []string{"hello"},
	})

	got := cs.Target()
	if got.Name != "verify" {
		t.Fatalf("Target().Name = %q; want %q", got.Name, "verify")
	}
	if got.Kind != SessionKindCapture {
		t.Fatalf("Target().Kind = %v; want %v", got.Kind, SessionKindCapture)
	}

	updated := SessionTarget{Name: "shell", Kind: SessionKindPTY, ID: "shell-1"}
	cs.SetTarget(updated)
	if got := cs.Target(); got != updated {
		t.Fatalf("Target() after SetTarget = %#v; want %#v", got, updated)
	}
}

func TestMuxSessionTargets_AreIndependent(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)

	active := SessionTarget{Name: "claude", Kind: SessionKindPTY}
	passthrough := SessionTarget{Name: "verify", Kind: SessionKindCapture}
	m.SetActiveTarget(active)
	m.SetPassthroughTarget(passthrough)

	if got := m.ActiveTarget(); got != active {
		t.Fatalf("ActiveTarget() = %#v; want %#v", got, active)
	}
	if got := m.PassthroughTarget(); got != passthrough {
		t.Fatalf("PassthroughTarget() = %#v; want %#v", got, passthrough)
	}

	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach error: %v", err)
	}
	if got := m.ActiveTarget(); got != active {
		t.Fatalf("ActiveTarget() after Attach = %#v; want %#v", got, active)
	}
	if got := m.PassthroughTarget(); got != passthrough {
		t.Fatalf("PassthroughTarget() after Attach = %#v; want %#v", got, passthrough)
	}

	child.Close()
	<-m.teeDone
}

func TestMuxSessionTargets_ResetOnDetach(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)

	active := SessionTarget{Name: "claude", Kind: SessionKindPTY}
	passthrough := SessionTarget{Name: "verify", Kind: SessionKindCapture}
	m.SetActiveTarget(active)
	m.SetPassthroughTarget(passthrough)

	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach error: %v", err)
	}
	child.Close()
	<-m.teeDone

	if err := m.Detach(); err != nil {
		t.Fatalf("Detach error: %v", err)
	}
	if got := m.ActiveTarget(); !got.IsZero() {
		t.Fatalf("ActiveTarget() after Detach = %#v; want zero", got)
	}
	if got := m.PassthroughTarget(); !got.IsZero() {
		t.Fatalf("PassthroughTarget() after Detach = %#v; want zero", got)
	}
}

func TestMuxSessionAdapter_RoundTripsState(t *testing.T) {
	var resizeRows, resizeCols uint16

	m := New(nil, io.Discard, -1)
	m.SetResizeFunc(func(rows, cols uint16) error {
		resizeRows = rows
		resizeCols = cols
		return nil
	})

	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	defer func() {
		child.Close()
		<-m.teeDone
	}()

	session := m.Session()
	session.SetTarget(SessionTarget{Name: "claude", ID: "claude-1"})
	if got := session.Target(); got.Kind != SessionKindPTY || got.Name != "claude" || got.ID != "claude-1" {
		t.Fatalf("Target() = %#v; want PTY claude target", got)
	}

	if _, err := session.Write([]byte("hello")); err != nil {
		t.Fatalf("Write: %v", err)
	}

	deadline := time.After(2 * time.Second)
	for {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for mux output, got %q", session.Output())
		default:
		}
		if strings.Contains(session.Output(), "hello") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := session.Resize(12, 34); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if resizeRows != 12 || resizeCols != 34 {
		t.Fatalf("resize = %dx%d; want 12x34", resizeRows, resizeCols)
	}
}

func TestMuxSession_Done_NoChild(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)
	session := m.Session()

	// Done() should return an already-closed channel when no child is attached.
	select {
	case <-session.Done():
		// Expected: channel is already closed.
	default:
		t.Fatal("Done() should be closed when no child is attached")
	}
}

func TestMuxSession_IsRunning_NoChild(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)
	session := m.Session()

	if session.IsRunning() {
		t.Fatal("IsRunning() should be false when no child is attached")
	}
}

func TestMuxSession_Done_WithChild(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)

	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	session := m.Session()

	// IsRunning should be true with a child attached.
	if !session.IsRunning() {
		t.Fatal("IsRunning() should be true when child is attached")
	}

	// Done() should NOT be closed while child is attached.
	select {
	case <-session.Done():
		t.Fatal("Done() should not be closed while child is attached")
	default:
		// Expected: channel is open.
	}

	// Close the child — this triggers EOF and Done().
	child.Close()

	// Wait for the tee goroutine to finish and close childEOF.
	select {
	case <-session.Done():
		// Expected: channel is now closed.
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Done() to close after child exit")
	}
}

func TestMuxSession_Done_AfterDetach(t *testing.T) {
	var stdin, stdout bytes.Buffer
	m := New(&stdin, &stdout, -1)

	child := newMockChild()
	if err := m.Attach(child); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	child.Close()
	<-m.teeDone

	if err := m.Detach(); err != nil {
		t.Fatalf("Detach: %v", err)
	}

	session := m.Session()

	// After detach, Done() should return a closed channel.
	select {
	case <-session.Done():
		// Expected: already closed.
	default:
		t.Fatal("Done() should be closed after Detach")
	}

	if session.IsRunning() {
		t.Fatal("IsRunning() should be false after Detach")
	}
}
