package vt

import (
	"fmt"
	"strings"
	"testing"
)

// The resize semantics of the scrolling region. A child that set a
// full-height region on the old geometry (bubbletea does exactly this) must
// be able to repaint the grown screen; a preserved stale region silently
// confines its CRLF-driven repaint to the old bounds.

func TestScreen_ResizeResetsScrollRegionOnHeightChange(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetScrollRegion(5, 20)
	if s.ScrollTop != 5 || s.ScrollBot != 20 {
		t.Fatalf("setup: region = %d-%d, want 5-20", s.ScrollTop, s.ScrollBot)
	}

	s.Resize(48, 80)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Fatalf("after growing: region = %d-%d, want 0-0 (full screen)", s.ScrollTop, s.ScrollBot)
	}
	if got := s.CurrentScrollRegion(); got != "1;48" {
		t.Fatalf("after growing: CurrentScrollRegion = %q, want %q", got, "1;48")
	}

	s.SetScrollRegion(1, 24)
	s.Resize(12, 80)
	if s.ScrollTop != 0 || s.ScrollBot != 0 {
		t.Fatalf("after shrinking: region = %d-%d, want 0-0 (full screen)", s.ScrollTop, s.ScrollBot)
	}
}

func TestScreen_ResizeKeepsScrollRegionOnEqualHeight(t *testing.T) {
	s := NewScreen(24, 80)
	s.SetScrollRegion(5, 20)

	// A width-only change must not disturb the region (tmux resets the
	// region only when the height changes).
	s.Resize(24, 120)
	if s.ScrollTop != 5 || s.ScrollBot != 20 {
		t.Fatalf("width-only resize: region = %d-%d, want 5-20", s.ScrollTop, s.ScrollBot)
	}
}

func TestVTerm_ResizedChildRepaintFillsNewScreen(t *testing.T) {
	v := NewVTerm(24, 80)

	// The child sets a full-height region on the 24-row screen, paints its
	// first frame, then the terminal grows.
	if _, err := v.Write([]byte("\x1b[1;24r")); err != nil {
		t.Fatalf("Write region: %v", err)
	}
	for i := 1; i <= 24; i++ {
		line := fmt.Sprintf("old-%02d", i)
		if i < 24 {
			line += "\r\n"
		}
		if _, err := v.Write([]byte(line)); err != nil {
			t.Fatalf("Write first frame: %v", err)
		}
	}
	v.Resize(48, 80)

	// The child repaints the way bubbletea does: home + clear, then rows
	// joined by CRLF and no trailing newline after the last row.
	if _, err := v.Write([]byte("\x1b[H\x1b[2J")); err != nil {
		t.Fatalf("Write home/clear: %v", err)
	}
	for i := 1; i <= 48; i++ {
		line := fmt.Sprintf("new-%02d", i)
		if i < 48 {
			line += "\r\n"
		}
		if _, err := v.Write([]byte(line)); err != nil {
			t.Fatalf("Write second frame: %v", err)
		}
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	scr := v.active
	if got := rowString(scr, 0); !strings.HasPrefix(got, "new-01") {
		t.Errorf("row 0 = %q, want the new frame's first row", strings.TrimRight(got, " "))
	}
	if got := rowString(scr, 47); !strings.HasPrefix(got, "new-48") {
		t.Errorf("row 47 = %q, want the new frame's last row (it must not be confined to the old region)", strings.TrimRight(got, " "))
	}
}
