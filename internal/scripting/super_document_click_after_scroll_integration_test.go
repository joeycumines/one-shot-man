//go:build unix

package scripting

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
)

func waitForBufferContains(ctx context.Context, cp *termtest.Console, target string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if strings.Contains(cp.String(), target) {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for %q", target)
		case <-ticker.C:
		}
	}
}

// TestSuperDocument_ClickAfterAutoScrollPlacesCursorCorrectly reproduces the
// real user scenario that triggered the stale viewport bug: type until the
// input viewport auto-scrolls, immediately click the visible text, and verify
// the cursor is placed where clicked by inserting a marker at the click
// position and asserting it shows up in the submitted document.
func TestSuperDocument_ClickAfterAutoScrollPlacesCursorCorrectly(t *testing.T) {
	if !isUnixPlatform() {
		t.Skip("Unix-only integration test")
	}

	binaryPath := buildTestBinary(t)
	env := newTestProcessEnv(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "super-document", "--interactive"),
		termtest.WithDefaultTimeout(30*time.Second),
		termtest.WithEnv(env),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	mouse := NewMouseTestAPI(t, cp)

	expect := func(snap termtest.Snapshot, target string, timeout time.Duration) {
		t.Helper()
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := cp.Expect(ctx, snap, termtest.Contains(target), fmt.Sprintf("wait for %q", target)); err != nil {
			t.Fatalf("Expected %q: %v\nBuffer: %q", target, err, cp.String())
		}
	}

	// Wait for initial render
	snap := cp.Snapshot()
	expect(snap, "Super-Document Builder", 15*time.Second)

	// Enter Add mode and focus textarea
	snap = cp.Snapshot()
	sendKey(t, cp, "a")
	expect(snap, "Content (multi-line):", 5*time.Second)
	// Focus textarea via click
	if err := mouse.ClickElement(ctx, "Content (multi-line):", 5*time.Second); err != nil {
		t.Fatalf("Failed to click on Content area: %v", err)
	}

	marker := "UNIQ-MARKER-CLICK-AFTER-SCROLL"
	// Type enough lines to force auto-scroll; put the unique marker on the last line.
	// 25 lines is sufficient to trigger auto-scroll in any reasonable terminal height
	// while keeping the test fast and deterministic.
	numLines := 25
	for i := 0; i < numLines; i++ {
		line := fmt.Sprintf("line-%03d", i)
		if i == numLines-1 {
			line = line + " " + marker
		}
		for _, ch := range line {
			sendKey(t, cp, string(ch))
			time.Sleep(5 * time.Millisecond) // Slightly slower typing to improve reliability
		}
		sendKey(t, cp, "\r")
		// Slightly longer delay after each line to let the terminal fully process
		time.Sleep(30 * time.Millisecond)
	}

	// Wait for the marker to appear in the buffer before attempting to click
	// This ensures typing is complete and the content is rendered
	if err := waitForBufferContains(ctx, cp, marker, 20*time.Second); err != nil {
		t.Fatalf("Marker never appeared after typing: %v\nBuffer: %q", err, cp.String())
	}

	// Now attempt to click the visible marker
	if err := mouse.ClickElement(ctx, marker, 10*time.Second); err != nil {
		t.Fatalf("Failed to click marker element: %v", err)
	}

	// Insert text at click position and submit form
	insert := "-INSERTED-"
	for _, ch := range insert {
		sendKey(t, cp, string(ch))
		time.Sleep(4 * time.Millisecond)
	}

	// Tab to Submit and press Enter
	sendKey(t, cp, "\t")
	time.Sleep(50 * time.Millisecond)
	snap = cp.Snapshot()
	sendKey(t, cp, "\r")

	// Verify the submitted document contains both the marker and the inserted text
	expect(snap, "Documents: 1", 5*time.Second)
	// Wait for marker & insert anywhere in the buffer (robust against snapshot boundaries)
	if err := waitForBufferContains(ctx, cp, marker, 5*time.Second); err != nil {
		t.Fatalf("Expected %q in buffer: %v\nBuffer: %q", marker, err, cp.String())
	}
	if err := waitForBufferContains(ctx, cp, insert, 5*time.Second); err != nil {
		t.Fatalf("Expected %q in buffer: %v\nBuffer: %q", insert, err, cp.String())
	}

	// Proximity sanity check: inserted text should appear after marker
	buf := cp.String()
	if idxMarker := strings.Index(buf, marker); idxMarker >= 0 {
		if idxInsert := strings.Index(buf, insert); idxInsert >= 0 {
			if idxInsert < idxMarker {
				t.Fatalf("Inserted text appears before marker in buffer - likely click targeted wrong line\nBuffer: %q", buf)
			}
		}
	}

	// Quit
	sendKey(t, cp, "q")
	if code, err := cp.WaitExit(ctx); err != nil || code != 0 {
		t.Fatalf("Expected exit code 0, got %d (err: %v)\nBuffer: %q", code, err, cp.String())
	}
}
