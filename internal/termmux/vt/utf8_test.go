package vt

import (
	"testing"
	"unicode/utf8"
)

func TestUTF8Accum_ASCII(t *testing.T) {
	var a UTF8Accum
	r, ok := a.Feed('A')
	if !ok || r != 'A' {
		t.Fatalf("ASCII: got (%U, %v), want ('A', true)", r, ok)
	}
	if a.Pending() {
		t.Fatal("Pending should be false after ASCII")
	}
}

func TestUTF8Accum_TwoByte(t *testing.T) {
	// é = U+00E9 = 0xC3, 0xA9
	var a UTF8Accum
	r, ok := a.Feed(0xC3)
	if ok {
		t.Fatalf("first byte should not complete: got (%U, true)", r)
	}
	if !a.Pending() {
		t.Fatal("Pending should be true after first byte")
	}
	r, ok = a.Feed(0xA9)
	if !ok || r != 'é' {
		t.Fatalf("2-byte: got (%U, %v), want ('é', true)", r, ok)
	}
	if a.Pending() {
		t.Fatal("Pending should be false after completion")
	}
}

func TestUTF8Accum_ThreeByte(t *testing.T) {
	// 漢 = U+6F22 = 0xE6, 0xBC, 0xA2
	var a UTF8Accum
	for i, b := range []byte{0xE6, 0xBC} {
		r, ok := a.Feed(b)
		if ok {
			t.Fatalf("byte %d should not complete: got (%U, true)", i, r)
		}
	}
	r, ok := a.Feed(0xA2)
	if !ok || r != '漢' {
		t.Fatalf("3-byte: got (%U, %v), want ('漢', true)", r, ok)
	}
}

func TestUTF8Accum_FourByte(t *testing.T) {
	// 𝕊 = U+1D54A = 0xF0, 0x9D, 0x95, 0x8A
	var a UTF8Accum
	for i, b := range []byte{0xF0, 0x9D, 0x95} {
		r, ok := a.Feed(b)
		if ok {
			t.Fatalf("byte %d should not complete: got (%U, true)", i, r)
		}
	}
	r, ok := a.Feed(0x8A)
	if !ok || r != '𝕊' {
		t.Fatalf("4-byte: got (%U, %v), want ('𝕊', true)", r, ok)
	}
}

func TestUTF8Accum_TruncatedThenASCII(t *testing.T) {
	// Start a 3-byte sequence, then feed ASCII → RuneError, then
	// re-feed the ASCII byte.
	var a UTF8Accum
	_, _ = a.Feed(0xE6) // first byte of 漢
	_, _ = a.Feed(0xBC) // second byte

	// Feed ASCII 'X' while accumulating → RuneError (byte not consumed).
	r, ok := a.Feed('X')
	if !ok || r != utf8.RuneError {
		t.Fatalf("truncated: got (%U, %v), want (RuneError, true)", r, ok)
	}

	// Re-feed 'X' — should return normally.
	r, ok = a.Feed('X')
	if !ok || r != 'X' {
		t.Fatalf("re-feed: got (%U, %v), want ('X', true)", r, ok)
	}
}

func TestUTF8Accum_PendingDuringAccumulation(t *testing.T) {
	var a UTF8Accum
	if a.Pending() {
		t.Fatal("should not be pending initially")
	}
	_, _ = a.Feed(0xC3) // start 2-byte
	if !a.Pending() {
		t.Fatal("should be pending after start byte")
	}
	_, _ = a.Feed(0xA9) // complete
	if a.Pending() {
		t.Fatal("should not be pending after completion")
	}
}

func TestUTF8Accum_StrayContinuation(t *testing.T) {
	// A continuation byte (0x80-0xBF) outside accumulation is invalid.
	var a UTF8Accum
	r, ok := a.Feed(0x80)
	if !ok || r != utf8.RuneError {
		t.Fatalf("stray continuation: got (%U, %v), want (RuneError, true)", r, ok)
	}
}

func TestUTF8Accum_NewSequenceInterrupts(t *testing.T) {
	// Start a 2-byte sequence then feed a new start byte (>= 0xC0).
	var a UTF8Accum
	_, _ = a.Feed(0xC3) // start 2-byte

	// New start byte → RuneError (not consumed).
	r, ok := a.Feed(0xC3)
	if !ok || r != utf8.RuneError {
		t.Fatalf("new start: got (%U, %v), want (RuneError, true)", r, ok)
	}

	// Re-feed the start byte and its continuation.
	r, ok = a.Feed(0xC3)
	if ok {
		t.Fatalf("re-feed start should accumulate: got (%U, true)", r)
	}
	r, ok = a.Feed(0xA9)
	if !ok || r != 'é' {
		t.Fatalf("re-feed complete: got (%U, %v), want ('é', true)", r, ok)
	}
}
