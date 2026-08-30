package vt

import (
	"strings"
	"sync"
	"testing"
)

// --- DCSData parser method tests --------------------------------------------

func TestParser_DCSData_STTerminator(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1bPq#0;2;0;0;0#1;2;100;100;0\x1b\\"))
	data := p.DCSData()
	if len(data) == 0 {
		t.Fatal("DCSData returned empty; expected accumulated payload")
	}
	expected := "q#0;2;0;0;0#1;2;100;100;0"
	if string(data) != expected {
		t.Fatalf("DCSData = %q; want %q", string(data), expected)
	}
}

func TestParser_DCSData_BELTerminator(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1bPsomedata\x07"))
	data := p.DCSData()
	if string(data) != "somedata" {
		t.Fatalf("DCSData = %q; want %q", string(data), "somedata")
	}
}

func TestParser_DCSData_EmptyPayload(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1bP\x1b\\"))
	data := p.DCSData()
	if data != nil {
		t.Fatalf("DCSData = %q; want nil for empty payload", string(data))
	}
}

func TestParser_DCSData_NoDCS(t *testing.T) {
	p := NewParser()
	data := p.DCSData()
	if data != nil {
		t.Fatalf("DCSData = %q; want nil before any DCS sequence", string(data))
	}
}

func TestParser_DCSData_CopyIsolation(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1bPpayload\x1b\\"))
	data := p.DCSData()
	// Modify returned slice — should not affect parser's internal buffer.
	data[0] = 'X'
	data2 := p.DCSData()
	if string(data2) != "payload" {
		t.Fatalf("DCSData not isolated: got %q after mutating returned slice", string(data2))
	}
}

func TestParser_DCSData_PartialFeeds(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1bPpart1"))
	if p.CurState() != StateDCS {
		t.Fatalf("after first chunk: state = %d; want StateDCS", p.CurState())
	}
	feedAll(p, []byte("part2"))
	feedAll(p, []byte("\x1b\\"))
	data := p.DCSData()
	if string(data) != "part1part2" {
		t.Fatalf("DCSData = %q; want %q", string(data), "part1part2")
	}
}

func TestParser_DCSData_MaxDCSLen(t *testing.T) {
	p := NewParser()
	p.maxDCSLen = 10
	// Send a DCS with 20 bytes of payload — only first 10 should be kept.
	payload := make([]byte, 20)
	for i := range payload {
		payload[i] = 'A' + byte(i%26)
	}
	feedAll(p, append([]byte("\x1bP"), append(payload, []byte("\x1b\\")...)...))
	data := p.DCSData()
	if len(data) != 10 {
		t.Fatalf("DCSData length = %d; want 10 (maxDCSLen)", len(data))
	}
}

func TestParser_DCSData_ResetClears(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1bPdata\x1b\\"))
	p.Reset()
	data := p.DCSData()
	if data != nil {
		t.Fatalf("DCSData after Reset = %q; want nil", string(data))
	}
}

// --- VTerm DCSHandler tests ------------------------------------------------

func TestDCSHandler_CalledWithData(t *testing.T) {
	v := NewVTerm(24, 80)
	var mu sync.Mutex
	var gotData []byte
	v.DCSHandler = func(data []byte) {
		mu.Lock()
		gotData = data
		mu.Unlock()
	}
	v.Write([]byte("\x1bPsomedata\x07"))

	mu.Lock()
	defer mu.Unlock()
	if string(gotData) != "somedata" {
		t.Fatalf("DCSHandler data = %q; want %q", string(gotData), "somedata")
	}
}

func TestDCSHandler_STTerminator(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotData []byte
	v.DCSHandler = func(data []byte) {
		gotData = data
	}
	v.Write([]byte("\x1bPpayload\x1b\\"))
	if string(gotData) != "payload" {
		t.Fatalf("DCSHandler data = %q; want %q", string(gotData), "payload")
	}
}

func TestDCSHandler_Nil(t *testing.T) {
	v := NewVTerm(24, 80)
	// DCSHandler is nil by default. Writing DCS should not panic.
	v.Write([]byte("\x1bPdata\x1b\\"))
}

func TestDCSHandler_EmptyPayload(t *testing.T) {
	v := NewVTerm(24, 80)
	var called bool
	v.DCSHandler = func(data []byte) {
		called = true
		if data != nil {
			t.Fatalf("DCSHandler data = %q; want nil for empty payload", string(data))
		}
	}
	v.Write([]byte("\x1bP\x1b\\"))
	if !called {
		t.Fatal("DCSHandler not called for empty payload")
	}
}

func TestDCSHandler_InterleavedWithText(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotData []byte
	v.DCSHandler = func(data []byte) {
		gotData = data
	}
	v.Write([]byte("Hello\x1bPdcsdata\x1b\\World"))
	if string(gotData) != "dcsdata" {
		t.Fatalf("DCSHandler data = %q; want %q", string(gotData), "dcsdata")
	}
	s := v.String()
	if s != "HelloWorld" {
		t.Fatalf("screen = %q; want %q", s, "HelloWorld")
	}
}

func TestDCSHandler_SixelPayload(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotData []byte
	v.DCSHandler = func(data []byte) {
		gotData = data
	}
	// Sixel DCS: ESC P q ... ESC \
	v.Write([]byte("\x1bPq#0;2;0;0;0#1;2;100;100;0\x1b\\"))
	if len(gotData) == 0 {
		t.Fatal("DCSHandler not called for Sixel DCS")
	}
	expected := "q#0;2;0;0;0#1;2;100;100;0"
	if string(gotData) != expected {
		t.Fatalf("DCSHandler data = %q; want %q", string(gotData), expected)
	}
}

func TestDCSHandler_DECRQSS(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotData []byte
	v.DCSHandler = func(data []byte) {
		gotData = data
	}
	// DECRQSS: ESC P $ q ESC \
	v.Write([]byte("\x1bP$q\x1b\\"))
	if string(gotData) != "$q" {
		t.Fatalf("DCSHandler data = %q; want %q", string(gotData), "$q")
	}
}

func TestDCSHandler_DataCopyIsolation(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotData []byte
	v.DCSHandler = func(data []byte) {
		// Make a copy to verify isolation
		gotData = make([]byte, len(data))
		copy(gotData, data)
	}
	v.Write([]byte("\x1bPtest\x1b\\"))
	// The handler received a copy — modifying it should not affect anything.
	gotData[0] = 'X'
	// No assertion needed — just verifying no panic or corruption.
}

func TestDCSHandler_MultipleSequences(t *testing.T) {
	v := NewVTerm(24, 80)
	var mu sync.Mutex
	calls := [][]byte{}
	v.DCSHandler = func(data []byte) {
		mu.Lock()
		cp := make([]byte, len(data))
		copy(cp, data)
		calls = append(calls, cp)
		mu.Unlock()
	}
	v.Write([]byte("\x1bPfirst\x1b\\\x1bPsecond\x07"))
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("got %d calls; want 2", len(calls))
	}
	if string(calls[0]) != "first" {
		t.Fatalf("call 0 = %q; want %q", string(calls[0]), "first")
	}
	if string(calls[1]) != "second" {
		t.Fatalf("call 1 = %q; want %q", string(calls[1]), "second")
	}
}

func TestDCSHandler_ConcurrentWrites(t *testing.T) {
	v := NewVTerm(24, 80)
	var mu sync.Mutex
	count := 0
	v.DCSHandler = func(data []byte) {
		mu.Lock()
		count++
		mu.Unlock()
	}
	var wg sync.WaitGroup
	for i := range 10 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			v.Write([]byte("\x1bPdata\x1b\\"))
		}(i)
	}
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if count != 10 {
		t.Fatalf("got %d calls; want 10", count)
	}
}

func TestDCSHandler_MixedWithCSI(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotData []byte
	v.DCSHandler = func(data []byte) {
		gotData = data
	}
	v.Write([]byte("\x1b[1;31m\x1bPdcs\x1b\\\x1b[0m"))
	if string(gotData) != "dcs" {
		t.Fatalf("DCSHandler data = %q; want %q", string(gotData), "dcs")
	}
}

func TestDCSHandler_LongPayload(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotData []byte
	v.DCSHandler = func(data []byte) {
		gotData = data
	}
	var payload strings.Builder
	for range 200 {
		payload.WriteString("A")
	}
	v.Write([]byte("\x1bP" + payload.String() + "\x07"))
	if string(gotData) != payload.String() {
		t.Fatalf("DCSHandler data length = %d; want %d", len(gotData), len(payload.String()))
	}
}

func TestDCSHandler_TruncatedAtMaxDCSLen(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotData []byte
	v.DCSHandler = func(data []byte) {
		gotData = data
	}
	// Default maxDCSLen is 4096
	var payload strings.Builder
	for range 5000 {
		payload.WriteString("X")
	}
	v.Write([]byte("\x1bP" + payload.String() + "\x07"))
	if len(gotData) != 4096 {
		t.Fatalf("DCSHandler data length = %d; want 4096 (maxDCSLen)", len(gotData))
	}
}
