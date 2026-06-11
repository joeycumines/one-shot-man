package vt

import (
	"sync"
	"testing"
)

// --- OSCData parser method tests --------------------------------------------

func TestParser_OSCData_CodeAndPayload(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1b]0;My Title\x07"))
	code, data := p.OSCData()
	if code != 0 {
		t.Fatalf("code = %d; want 0", code)
	}
	if data != "My Title" {
		t.Fatalf("data = %q; want %q", data, "My Title")
	}
}

func TestParser_OSCData_WorkingDirectory(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1b]7;file:///home/user\x07"))
	code, data := p.OSCData()
	if code != 7 {
		t.Fatalf("code = %d; want 7", code)
	}
	if data != "file:///home/user" {
		t.Fatalf("data = %q; want %q", data, "file:///home/user")
	}
}

func TestParser_OSCData_Clipboard(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1b]52;c;SGVsbG8=\x07"))
	code, data := p.OSCData()
	if code != 52 {
		t.Fatalf("code = %d; want 52", code)
	}
	if data != "c;SGVsbG8=" {
		t.Fatalf("data = %q; want %q", data, "c;SGVsbG8=")
	}
}

func TestParser_OSCData_NoSemicolon(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1b]nospace\x07"))
	code, data := p.OSCData()
	if code != 0 {
		t.Fatalf("code = %d; want 0 (no semicolon)", code)
	}
	if data != "nospace" {
		t.Fatalf("data = %q; want %q", data, "nospace")
	}
}

func TestParser_OSCData_MalformedCode(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1b]abc;payload\x07"))
	code, data := p.OSCData()
	if code != 0 {
		t.Fatalf("code = %d; want 0 (malformed code)", code)
	}
	if data != "abc;payload" {
		t.Fatalf("data = %q; want %q", data, "abc;payload")
	}
}

func TestParser_OSCData_EmptyPayload(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1b]0;\x07"))
	code, data := p.OSCData()
	if code != 0 {
		t.Fatalf("code = %d; want 0", code)
	}
	if data != "" {
		t.Fatalf("data = %q; want empty string", data)
	}
}

func TestParser_OSCData_EmptyBuffer(t *testing.T) {
	p := NewParser()
	code, data := p.OSCData()
	if code != 0 {
		t.Fatalf("code = %d; want 0", code)
	}
	if data != "" {
		t.Fatalf("data = %q; want empty string", data)
	}
}

func TestParser_OSCData_MultiDigitCode(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1b]1337;SetBadge=text\x07"))
	code, data := p.OSCData()
	if code != 1337 {
		t.Fatalf("code = %d; want 1337", code)
	}
	if data != "SetBadge=text" {
		t.Fatalf("data = %q; want %q", data, "SetBadge=text")
	}
}

func TestParser_OSCData_DataWithSemicolons(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1b]8;;https://example.com\x07"))
	code, data := p.OSCData()
	if code != 8 {
		t.Fatalf("code = %d; want 8", code)
	}
	if data != ";https://example.com" {
		t.Fatalf("data = %q; want %q", data, ";https://example.com")
	}
}

func TestParser_OSCData_STTerminator(t *testing.T) {
	p := NewParser()
	feedAll(p, []byte("\x1b]2;Title via ST\x1b\\"))
	code, data := p.OSCData()
	if code != 2 {
		t.Fatalf("code = %d; want 2", code)
	}
	if data != "Title via ST" {
		t.Fatalf("data = %q; want %q", data, "Title via ST")
	}
}

// --- VTerm OSCHandler tests ------------------------------------------------

func TestOSCHandler_TitleOSC0(t *testing.T) {
	v := NewVTerm(24, 80)
	var mu sync.Mutex
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		mu.Lock()
		gotCode = code
		gotData = data
		mu.Unlock()
	}
	v.Write([]byte("\x1b]0;My Window\x07"))

	mu.Lock()
	defer mu.Unlock()
	if gotCode != 0 {
		t.Fatalf("code = %d; want 0", gotCode)
	}
	if gotData != "My Window" {
		t.Fatalf("data = %q; want %q", gotData, "My Window")
	}
}

func TestOSCHandler_TitleOSC2(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		gotCode = code
		gotData = data
	}
	v.Write([]byte("\x1b]2;XTerm Title\x07"))

	if gotCode != 2 {
		t.Fatalf("code = %d; want 2", gotCode)
	}
	if gotData != "XTerm Title" {
		t.Fatalf("data = %q; want %q", gotData, "XTerm Title")
	}
}

func TestOSCHandler_WorkingDirectory(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		gotCode = code
		gotData = data
	}
	v.Write([]byte("\x1b]7;file:///home/user/projects\x07"))

	if gotCode != 7 {
		t.Fatalf("code = %d; want 7", gotCode)
	}
	if gotData != "file:///home/user/projects" {
		t.Fatalf("data = %q; want %q", gotData, "file:///home/user/projects")
	}
}

func TestOSCHandler_Clipboard(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		gotCode = code
		gotData = data
	}
	v.Write([]byte("\x1b]52;c;SGVsbG8gV29ybGQ=\x07"))

	if gotCode != 52 {
		t.Fatalf("code = %d; want 52", gotCode)
	}
	if gotData != "c;SGVsbG8gV29ybGQ=" {
		t.Fatalf("data = %q; want %q", gotData, "c;SGVsbG8gV29ybGQ=")
	}
}

func TestOSCHandler_MultipleSequences(t *testing.T) {
	v := NewVTerm(24, 80)
	var mu sync.Mutex
	calls := []struct{ code int; data string }{}
	v.OSCHandler = func(code int, data string) {
		mu.Lock()
		calls = append(calls, struct{ code int; data string }{code, data})
		mu.Unlock()
	}

	// Send three OSC sequences in one Write call.
	v.Write([]byte("\x1b]0;Title1\x07\x1b]7;file:///tmp\x07\x1b]52;c;AAAA\x07"))

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 3 {
		t.Fatalf("got %d calls; want 3", len(calls))
	}
	if calls[0].code != 0 || calls[0].data != "Title1" {
		t.Fatalf("call 0: code=%d data=%q; want code=0 data=%q", calls[0].code, calls[0].data, "Title1")
	}
	if calls[1].code != 7 || calls[1].data != "file:///tmp" {
		t.Fatalf("call 1: code=%d data=%q; want code=7 data=%q", calls[1].code, calls[1].data, "file:///tmp")
	}
	if calls[2].code != 52 || calls[2].data != "c;AAAA" {
		t.Fatalf("call 2: code=%d data=%q; want code=52 data=%q", calls[2].code, calls[2].data, "c;AAAA")
	}
}

func TestOSCHandler_Nil(t *testing.T) {
	v := NewVTerm(24, 80)
	// OSCHandler is nil by default. Writing OSC should not panic.
	v.Write([]byte("\x1b]0;Title\x07"))
	// No assertion needed — the test passes if it doesn't panic.
}

func TestOSCHandler_InterleavedWithText(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		gotCode = code
		gotData = data
	}
	// Text before and after OSC.
	v.Write([]byte("Hello\x1b]2;Title\x07World"))

	if gotCode != 2 {
		t.Fatalf("code = %d; want 2", gotCode)
	}
	if gotData != "Title" {
		t.Fatalf("data = %q; want %q", gotData, "Title")
	}
	// Verify text was rendered to screen.
	s := v.String()
	if s != "HelloWorld" {
		t.Fatalf("screen = %q; want %q", s, "HelloWorld")
	}
}

func TestOSCHandler_MixedWithCSI(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		gotCode = code
		gotData = data
	}
	// CSI before OSC, CSI after.
	v.Write([]byte("\x1b[1;31m\x1b]0;Red Title\x07\x1b[0m"))

	if gotCode != 0 {
		t.Fatalf("code = %d; want 0", gotCode)
	}
	if gotData != "Red Title" {
		t.Fatalf("data = %q; want %q", gotData, "Red Title")
	}
}

func TestOSCHandler_UnknownCode(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		gotCode = code
		gotData = data
	}
	// OSC 777 is used by some terminals but not one we special-case.
	v.Write([]byte("\x1b]777;notify;title;body\x07"))

	if gotCode != 777 {
		t.Fatalf("code = %d; want 777", gotCode)
	}
	if gotData != "notify;title;body" {
		t.Fatalf("data = %q; want %q", gotData, "notify;title;body")
	}
}

func TestOSCHandler_EmptyData(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		gotCode = code
		gotData = data
	}
	v.Write([]byte("\x1b]0;\x07"))

	if gotCode != 0 {
		t.Fatalf("code = %d; want 0", gotCode)
	}
	if gotData != "" {
		t.Fatalf("data = %q; want empty string", gotData)
	}
}

func TestOSCHandler_STTerminator(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		gotCode = code
		gotData = data
	}
	v.Write([]byte("\x1b]2;Title via ST\x1b\\"))

	if gotCode != 2 {
		t.Fatalf("code = %d; want 2", gotCode)
	}
	if gotData != "Title via ST" {
		t.Fatalf("data = %q; want %q", gotData, "Title via ST")
	}
}

func TestOSCHandler_LongPayload(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		gotCode = code
		gotData = data
	}
	// Build a long OSC 7 payload.
	longPath := "file:///home/user/"
	for i := 0; i < 200; i++ {
		longPath += "subdir/"
	}
	v.Write([]byte("\x1b]7;" + longPath + "\x07"))

	if gotCode != 7 {
		t.Fatalf("code = %d; want 7", gotCode)
	}
	if gotData != longPath {
		t.Fatalf("data length = %d; want %d", len(gotData), len(longPath))
	}
}

func TestOSCHandler_TruncatedAtMaxOSCLen(t *testing.T) {
	v := NewVTerm(24, 80)
	var gotCode int
	var gotData string
	v.OSCHandler = func(code int, data string) {
		gotCode = code
		gotData = data
	}
	// Build a payload that exceeds the default maxOSCLen (4096).
	var payload string
	for i := 0; i < 5000; i++ {
		payload += "X"
	}
	v.Write([]byte("\x1b]0;" + payload + "\x07"))

	// The data should be truncated at maxOSCLen minus the "0;" prefix.
	if gotCode != 0 {
		t.Fatalf("code = %d; want 0", gotCode)
	}
	// maxOSCLen=4096, but the "0;" prefix is also in oscBuf, so data is
	// maxOSCLen - 2 = 4094 characters of the payload.
	expectedLen := 4094 // 4096 - len("0;")
	if len(gotData) != expectedLen {
		t.Fatalf("data length = %d; want %d (maxOSCLen - 2)", len(gotData), expectedLen)
	}
}

func TestOSCHandler_ConcurrentWrites(t *testing.T) {
	v := NewVTerm(24, 80)
	var mu sync.Mutex
	codes := []int{}
	v.OSCHandler = func(code int, data string) {
		mu.Lock()
		codes = append(codes, code)
		mu.Unlock()
	}

	// Multiple goroutines writing OSC sequences concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			oscCode := n % 3
			v.Write([]byte("\x1b]" + string(rune('0'+oscCode)) + ";data\x07"))
		}(i)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(codes) != 10 {
		t.Fatalf("got %d calls; want 10", len(codes))
	}
}
