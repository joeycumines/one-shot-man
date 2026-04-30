package fetch

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

// --- ReadableStream Go-level tests ---

func TestReadableStream_NewDefaults(t *testing.T) {
	t.Parallel()
	rs := NewReadableStream(io.NopCloser(strings.NewReader("x")), nil)
	if rs.Locked() {
		t.Fatal("new stream should not be locked")
	}
}

func TestReadableStream_GetReader_LocksStream(t *testing.T) {
	t.Parallel()
	rs := NewReadableStream(io.NopCloser(strings.NewReader("hello")), nil)

	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	if !rs.Locked() {
		t.Fatal("stream should be locked after GetReader")
	}

	// Second GetReader must fail.
	_, err = rs.GetReader()
	if err == nil {
		t.Fatal("expected error on second GetReader")
	}
	if !strings.Contains(err.Error(), "locked") {
		t.Errorf("error should mention locked: %v", err)
	}

	reader.ReleaseLock()
	if rs.Locked() {
		t.Fatal("stream should be unlocked after ReleaseLock")
	}
}

func TestReadableStream_ReadAll(t *testing.T) {
	t.Parallel()
	data := "hello, streaming world! This is a test payload."
	rs := NewReadableStream(io.NopCloser(strings.NewReader(data)), nil)

	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	var buf bytes.Buffer
	for {
		chunk, done, readErr := reader.Read()
		if readErr != nil {
			t.Fatalf("Read: %v", readErr)
		}
		if done {
			break
		}
		buf.Write(chunk)
	}
	if got := buf.String(); got != data {
		t.Errorf("got %q, want %q", got, data)
	}
}

func TestReadableStream_LargeBody_MultipleChunks(t *testing.T) {
	t.Parallel()
	// Create a body larger than one chunk (>64 KiB).
	data := strings.Repeat("ABCDEFGH", 10000) // 80,000 bytes
	rs := NewReadableStream(io.NopCloser(strings.NewReader(data)), nil)

	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	var buf bytes.Buffer
	chunkCount := 0
	for {
		chunk, done, readErr := reader.Read()
		if readErr != nil {
			t.Fatalf("Read: %v", readErr)
		}
		if done {
			break
		}
		chunkCount++
		buf.Write(chunk)
	}
	if got := buf.String(); got != data {
		t.Errorf("body mismatch: len(got)=%d, len(want)=%d", len(got), len(data))
	}
	if chunkCount < 2 {
		t.Errorf("expected multiple chunks for %d bytes, got %d", len(data), chunkCount)
	}
}

func TestReadableStream_EmptyBody(t *testing.T) {
	t.Parallel()
	rs := NewReadableStream(io.NopCloser(strings.NewReader("")), nil)

	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	_, done, readErr := reader.Read()
	if readErr != nil {
		t.Fatalf("Read: %v", readErr)
	}
	if !done {
		t.Fatal("expected done=true for empty body")
	}
}

func TestReadableStream_Cancel_BeforeRead(t *testing.T) {
	t.Parallel()
	rs := NewReadableStream(io.NopCloser(strings.NewReader("data")), nil)

	if err := rs.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	// GetReader should fail after cancel.
	_, err := rs.GetReader()
	if err == nil {
		t.Fatal("expected error after cancel")
	}
}

func TestReadableStream_Cancel_Double(t *testing.T) {
	t.Parallel()
	rs := NewReadableStream(io.NopCloser(strings.NewReader("data")), nil)
	if err := rs.Cancel(); err != nil {
		t.Fatalf("first Cancel: %v", err)
	}
	if err := rs.Cancel(); err != nil {
		t.Fatalf("second Cancel should not error: %v", err)
	}
}

func TestReadableStream_Cancel_WhileReading(t *testing.T) {
	t.Parallel()
	// Use a blocking reader — pipe that we control.
	pr, pw := io.Pipe()
	rs := NewReadableStream(pr, nil)

	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}

	// Write some data, then cancel.
	_, _ = pw.Write([]byte("chunk1"))

	// Read the first chunk.
	data, done, readErr := reader.Read()
	if readErr != nil {
		t.Fatalf("Read: %v", readErr)
	}
	if done {
		t.Fatal("unexpected done before cancel")
	}
	if string(data) != "chunk1" {
		t.Errorf("got %q, want %q", string(data), "chunk1")
	}

	// Cancel the stream while pump is blocked waiting for more data.
	if cancelErr := rs.Cancel(); cancelErr != nil {
		t.Fatalf("Cancel: %v", cancelErr)
	}
	pw.Close()

	// The next read should either get an error or done.
	_, done2, readErr2 := reader.Read()
	if readErr2 != nil {
		// Pipe closed error is acceptable.
		return
	}
	if !done2 {
		t.Fatal("expected done or error after cancel")
	}
}

func TestReadableStreamDefaultReader_ReleaseLock_Double(t *testing.T) {
	t.Parallel()
	rs := NewReadableStream(io.NopCloser(strings.NewReader("data")), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}

	reader.ReleaseLock()
	reader.ReleaseLock() // should not panic

	if rs.Locked() {
		t.Fatal("stream should be unlocked")
	}
}

func TestReadableStreamDefaultReader_ReadAfterRelease(t *testing.T) {
	t.Parallel()
	rs := NewReadableStream(io.NopCloser(strings.NewReader("data")), nil)
	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	reader.ReleaseLock()

	_, _, readErr := reader.Read()
	if readErr == nil {
		t.Fatal("expected error reading after release")
	}
	if !strings.Contains(readErr.Error(), "released") {
		t.Errorf("error should mention released: %v", readErr)
	}
}

func TestReadableStream_ReacquireReaderAfterRelease(t *testing.T) {
	t.Parallel()
	data := "reacquire"
	rs := NewReadableStream(io.NopCloser(strings.NewReader(data)), nil)

	r1, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}

	// Read partial data.
	chunk, _, readErr := r1.Read()
	if readErr != nil {
		t.Fatalf("Read: %v", readErr)
	}
	r1.ReleaseLock()

	// Acquiring a new reader should succeed (pump already started).
	r2, err := rs.GetReader()
	if err != nil {
		t.Fatalf("second GetReader: %v", err)
	}
	defer r2.ReleaseLock()

	// Read remaining data.
	var buf bytes.Buffer
	buf.Write(chunk) // include what r1 already read
	for {
		c, d, e := r2.Read()
		if e != nil {
			t.Fatalf("Read: %v", e)
		}
		if d {
			break
		}
		buf.Write(c)
	}
	if got := buf.String(); got != data {
		t.Errorf("got %q, want %q", got, data)
	}
}

// errReader returns err on every Read call.
type errReader struct {
	err error
}

func (r *errReader) Read([]byte) (int, error) { return 0, r.err }
func (r *errReader) Close() error             { return nil }

func TestReadableStream_SourceError(t *testing.T) {
	t.Parallel()
	testErr := errors.New("synthetic read error")
	rs := NewReadableStream(&errReader{err: testErr}, nil)

	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	_, done, readErr := reader.Read()
	if done {
		t.Fatal("should not be done on error")
	}
	if readErr == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(readErr.Error(), "synthetic read error") {
		t.Errorf("unexpected error: %v", readErr)
	}
}

func TestReadableStream_ConcurrentReads(t *testing.T) {
	t.Parallel()
	// This tests that the bounded channel does not cause deadlocks
	// when pump is writing and consumer is reading concurrently.
	data := strings.Repeat("X", 200000) // ~200 KB
	rs := NewReadableStream(io.NopCloser(strings.NewReader(data)), nil)

	reader, err := rs.GetReader()
	if err != nil {
		t.Fatalf("GetReader: %v", err)
	}
	defer reader.ReleaseLock()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var total int

	// Read in a goroutine to simulate async consumer.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			chunk, done, readErr := reader.Read()
			if readErr != nil {
				t.Errorf("Read: %v", readErr)
				return
			}
			if done {
				return
			}
			mu.Lock()
			total += len(chunk)
			mu.Unlock()
		}
	}()
	wg.Wait()

	if total != len(data) {
		t.Errorf("total bytes = %d, want %d", total, len(data))
	}
}

func TestReadableStream_GetReader_AfterClosed(t *testing.T) {
	t.Parallel()
	rs := NewReadableStream(io.NopCloser(strings.NewReader("data")), nil)
	_ = rs.Cancel()

	_, err := rs.GetReader()
	if err == nil {
		t.Fatal("expected error on GetReader after close")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("error should mention closed: %v", err)
	}
}
