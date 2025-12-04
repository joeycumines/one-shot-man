package testutil

import (
	"regexp"
	"strings"
	"testing"
)

func extractSafeName(tid, prefix string) string {
	// tid is prefix-safeName-ID
	// Remove the leading `prefix-` then strip trailing -ID using the last dash.
	if !strings.HasPrefix(tid, prefix+"-") {
		return ""
	}
	rest := strings.TrimPrefix(tid, prefix+"-")
	lastDash := strings.LastIndex(rest, "-")
	if lastDash <= 0 {
		return ""
	}
	return rest[:lastDash]
}

func TestNewTestSessionID_ShortName(t *testing.T) {
	id := NewTestSessionID("pfx", "Short/SubTest")
	if !strings.HasPrefix(id, "pfx-") {
		t.Fatalf("expected prefix; got %q", id)
	}
	safe := extractSafeName(id, "pfx")
	if strings.Contains(safe, "/") {
		t.Fatalf("safe segment contains slash: %q", safe)
	}
	if len(safe) == 0 {
		t.Fatalf("safe segment empty: %q", id)
	}
}

func TestNewTestSessionID_LongName_TruncatesAndAppendsHash(t *testing.T) {
	var parts []string
	for i := 0; i < 200; i++ {
		parts = append(parts, "segment")
	}
	longName := strings.Join(parts, "/")

	id := NewTestSessionID("longpfx", longName)
	safe := extractSafeName(id, "longpfx")

	if len(safe) > 64 {
		t.Fatalf("safe name too long: %d bytes (max 64): %q", len(safe), safe)
	}

	// When truncation happens we append a short hex hash suffix (8 hex chars)
	matched, err := regexp.MatchString(`-[0-9a-f]{8}$`, safe)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatalf("expected safe name to end with -<8hex> when truncated; got %q", safe)
	}
}
