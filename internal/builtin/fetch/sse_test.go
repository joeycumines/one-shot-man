package fetch

import (
	"io"
	"strings"
	"testing"
)

func TestSSEParser_SingleEvent(t *testing.T) {
	t.Parallel()
	input := "data: hello world\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, done, parseErr := parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if done {
		t.Fatal("unexpected done")
	}
	if ev.Event != "message" {
		t.Errorf("Event = %q, want %q", ev.Event, "message")
	}
	if ev.Data != "hello world" {
		t.Errorf("Data = %q, want %q", ev.Data, "hello world")
	}

	// Should be done after the single event.
	_, done, parseErr = parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if !done {
		t.Fatal("expected done after single event")
	}
}

func TestSSEParser_MultipleEvents(t *testing.T) {
	t.Parallel()
	input := "data: first\n\ndata: second\n\ndata: third\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	var events []string
	for {
		ev, done, parseErr := parser.Next()
		if parseErr != nil {
			t.Fatalf("Next: %v", parseErr)
		}
		if done {
			break
		}
		events = append(events, ev.Data)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}
	want := []string{"first", "second", "third"}
	for i, w := range want {
		if events[i] != w {
			t.Errorf("events[%d] = %q, want %q", i, events[i], w)
		}
	}
}

func TestSSEParser_CustomEventType(t *testing.T) {
	t.Parallel()
	input := "event: update\ndata: payload\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, done, parseErr := parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if done {
		t.Fatal("unexpected done")
	}
	if ev.Event != "update" {
		t.Errorf("Event = %q, want %q", ev.Event, "update")
	}
	if ev.Data != "payload" {
		t.Errorf("Data = %q, want %q", ev.Data, "payload")
	}
}

func TestSSEParser_MultiLineData(t *testing.T) {
	t.Parallel()
	input := "data: line1\ndata: line2\ndata: line3\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, done, parseErr := parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if done {
		t.Fatal("unexpected done")
	}
	if ev.Data != "line1\nline2\nline3" {
		t.Errorf("Data = %q, want %q", ev.Data, "line1\nline2\nline3")
	}
}

func TestSSEParser_IDField(t *testing.T) {
	t.Parallel()
	input := "id: 42\ndata: with-id\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, _, _ := parser.Next()
	if ev.ID != "42" {
		t.Errorf("ID = %q, want %q", ev.ID, "42")
	}
}

func TestSSEParser_IDPersistsAcrossEvents(t *testing.T) {
	t.Parallel()
	input := "id: 1\ndata: first\n\ndata: second\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev1, _, _ := parser.Next()
	if ev1.ID != "1" {
		t.Errorf("first event ID = %q, want %q", ev1.ID, "1")
	}
	ev2, _, _ := parser.Next()
	if ev2.ID != "1" {
		t.Errorf("second event ID = %q, want %q (should persist)", ev2.ID, "1")
	}
}

func TestSSEParser_CommentLines(t *testing.T) {
	t.Parallel()
	input := ": this is a comment\ndata: visible\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, done, parseErr := parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if done {
		t.Fatal("unexpected done")
	}
	if ev.Data != "visible" {
		t.Errorf("Data = %q, want %q", ev.Data, "visible")
	}
}

func TestSSEParser_CRLFDelimiters(t *testing.T) {
	t.Parallel()
	input := "data: crlf\r\n\r\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, done, parseErr := parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if done {
		t.Fatal("unexpected done")
	}
	if ev.Data != "crlf" {
		t.Errorf("Data = %q, want %q", ev.Data, "crlf")
	}
}

func TestSSEParser_EmptyStream(t *testing.T) {
	t.Parallel()
	rs := NewReadableStream(io.NopCloser(strings.NewReader("")), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	_, done, parseErr := parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if !done {
		t.Fatal("expected done for empty stream")
	}
}

func TestSSEParser_NoTrailingNewline(t *testing.T) {
	t.Parallel()
	// Stream ends without a trailing blank line — should still flush.
	input := "data: unterminated"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, done, parseErr := parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if done {
		t.Fatal("expected event from flush, not done")
	}
	if ev.Data != "unterminated" {
		t.Errorf("Data = %q, want %q", ev.Data, "unterminated")
	}
}

func TestSSEParser_DataWithColonInValue(t *testing.T) {
	t.Parallel()
	input := "data: key:value:extra\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, _, _ := parser.Next()
	if ev.Data != "key:value:extra" {
		t.Errorf("Data = %q, want %q", ev.Data, "key:value:extra")
	}
}

func TestSSEParser_FieldWithNoColon(t *testing.T) {
	t.Parallel()
	// A line with no colon is treated as a field name with empty value.
	// "data" with no colon means empty data line.
	input := "data\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, done, parseErr := parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if done {
		t.Fatal("unexpected done")
	}
	if ev.Data != "" {
		t.Errorf("Data = %q, want empty string", ev.Data)
	}
}

func TestSSEParser_LeadingSpaceStripped(t *testing.T) {
	t.Parallel()
	// Per spec, a single leading space after the colon is stripped.
	input := "data: hello\ndata:  two-spaces\ndata:no-space\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, _, _ := parser.Next()
	// "data: hello" → "hello"
	// "data:  two-spaces" → " two-spaces" (only ONE leading space stripped)
	// "data:no-space" → "no-space"
	want := "hello\n two-spaces\nno-space"
	if ev.Data != want {
		t.Errorf("Data = %q, want %q", ev.Data, want)
	}
}

func TestSSEParser_BlankLinesWithoutData(t *testing.T) {
	t.Parallel()
	// Multiple blank lines should not produce events if no data was accumulated.
	input := "\n\n\ndata: after-blanks\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, done, parseErr := parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if done {
		t.Fatal("unexpected done")
	}
	if ev.Data != "after-blanks" {
		t.Errorf("Data = %q, want %q", ev.Data, "after-blanks")
	}
}

func TestSSEParser_RetryField(t *testing.T) {
	t.Parallel()
	input := "retry: 3000\ndata: with-retry\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, done, parseErr := parser.Next()
	if parseErr != nil {
		t.Fatalf("Next: %v", parseErr)
	}
	if done {
		t.Fatal("unexpected done")
	}
	if ev.Data != "with-retry" {
		t.Errorf("Data = %q, want %q", ev.Data, "with-retry")
	}
	if ev.Retry != 3000 {
		t.Errorf("Retry = %d, want 3000", ev.Retry)
	}
}

func TestSSEParser_IDWithNullByte(t *testing.T) {
	t.Parallel()
	// Per spec, id fields containing null bytes are ignored.
	input := "id: bad\x00id\ndata: test\n\n"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(input)), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)
	ev, _, _ := parser.Next()
	if ev.ID != "" {
		t.Errorf("ID = %q, want empty (null byte should be rejected)", ev.ID)
	}
}

func TestSSEParser_CRLFAcrossChunkBoundary(t *testing.T) {
	t.Parallel()
	// Simulate a \r\n sequence split across two chunks: the first
	// chunk ends with \r and the second starts with \n.  Without
	// lastCharWasCR handling, this would produce a phantom blank line.
	chunk1 := "data: hello\r"
	chunk2 := "\n\ndata: world\n\n"

	pr, pw := io.Pipe()
	rs := NewReadableStream(pr, nil)

	go func() {
		_, _ = pw.Write([]byte(chunk1))
		_, _ = pw.Write([]byte(chunk2))
		pw.Close()
	}()

	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	parser := NewSSEParser(reader)

	ev1, done1, err1 := parser.Next()
	if err1 != nil {
		t.Fatalf("Next: %v", err1)
	}
	if done1 {
		t.Fatal("unexpected done for first event")
	}
	if ev1.Data != "hello" {
		t.Errorf("first event Data = %q, want %q", ev1.Data, "hello")
	}

	ev2, done2, err2 := parser.Next()
	if err2 != nil {
		t.Fatalf("Next: %v", err2)
	}
	if done2 {
		t.Fatal("unexpected done for second event")
	}
	if ev2.Data != "world" {
		t.Errorf("second event Data = %q, want %q", ev2.Data, "world")
	}

	_, done3, err3 := parser.Next()
	if err3 != nil {
		t.Fatalf("Next: %v", err3)
	}
	if !done3 {
		t.Fatal("expected done after all events")
	}
}
