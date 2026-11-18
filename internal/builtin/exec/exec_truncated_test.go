package exec

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// errReader returns the payload prefix and then fails with cause, modeling a
// PTY/pipe read error (e.g. EIO) after partial output. It also implements
// Write so it satisfies commandPipeEndpoint for the reader side of the pipe
// factory.
type prefixThenErrReader struct {
	payload []byte
	cause   error
	read    bool
}

func (r *prefixThenErrReader) Read(buf []byte) (int, error) {
	if !r.read {
		r.read = true
		return copy(buf, r.payload), nil
	}
	return 0, r.cause
}

func (r *prefixThenErrReader) Write(data []byte) (int, error) { return len(data), nil }
func (r *prefixThenErrReader) Close() error                   { return nil }

// closeErrAfterEOFReader reaches EOF cleanly and then fails Close, modeling a
// close failure after a complete read.
type closeErrAfterEOFReader struct {
	closeErr error
}

func (r *closeErrAfterEOFReader) Read([]byte) (int, error)       { return 0, io.EOF }
func (r *closeErrAfterEOFReader) Write(data []byte) (int, error) { return len(data), nil }
func (r *closeErrAfterEOFReader) Close() error                   { return r.closeErr }

// discardWriter accepts writes and closes cleanly; the child's output is
// consumed by the custom readers above.
type discardWriter struct{}

func (*discardWriter) Write(data []byte) (int, error) { return len(data), nil }
func (*discardWriter) Read([]byte) (int, error)       { return 0, io.EOF }
func (*discardWriter) Close() error                   { return nil }

// TestRunExec_NonEOFReadErrorWithKnownExitCodeIsTruncated pins review-15 P1#1B:
// a child that exits with a known nonzero code while a reader returns a prefix
// followed by a non-EOF error must surface a structural truncated flag, so the
// pr-split verifier classifies the capture as unknown instead of a
// deterministic failure. Before the fix, complete was "both goroutines
// returned" and the read/close errors were joined, so the result carried only
// code 7 + an error message.
func TestRunExec_NonEOFReadErrorWithKnownExitCodeIsTruncated(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	eio := errors.New("input/output error")
	stdout := &prefixThenErrReader{payload: []byte("partial-output\n"), cause: eio}
	stderr := &prefixThenErrReader{payload: nil, cause: io.EOF}
	calls := 0
	pipeFactory := func() (commandPipeEndpoint, commandPipeEndpoint, error) {
		calls++
		if calls == 1 {
			return stdout, &discardWriter{}, nil
		}
		return stderr, &discardWriter{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command, args := exitCodeCommand(7)
	result := runExecCommand(ctx, command, args, pipeFactory, outputEOFCeiling)

	if result["code"] != 7 {
		t.Fatalf("code = %v, want 7 (the real exit status must survive)", result["code"])
	}
	if result["error"] != true {
		t.Fatalf("error = %v, want true (the read failure is still reported)", result["error"])
	}
	if result["truncated"] != true {
		t.Fatalf("truncated = %v, want true: a prefix+EIO read on a known exit code is an incomplete capture", result["truncated"])
	}
	if got := result["stdout"]; got != "partial-output\n" {
		t.Fatalf("stdout = %q, want the captured prefix", got)
	}
	if msg, _ := result["message"].(string); !strings.Contains(msg, eio.Error()) {
		t.Fatalf("message = %q, want it to retain the read error %q", msg, eio.Error())
	}
}

// TestRunExec_CloseErrorAfterEOFStaysComplete pins the complementary case: a
// reader that reaches EOF and then fails Close must NOT be classified as
// truncated — every byte was captured.
func TestRunExec_CloseErrorAfterEOFStaysComplete(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	closeErr := errors.New("close after eof failed")
	stdout := &closeErrAfterEOFReader{closeErr: closeErr}
	stderr := &closeErrAfterEOFReader{closeErr: nil}
	calls := 0
	pipeFactory := func() (commandPipeEndpoint, commandPipeEndpoint, error) {
		calls++
		if calls == 1 {
			return stdout, &discardWriter{}, nil
		}
		return stderr, &discardWriter{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command, args := exitCodeCommand(0)
	result := runExecCommand(ctx, command, args, pipeFactory, outputEOFCeiling)

	// The writer side fails to close (closeErr is attached to the READER's
	// Close, which runExecCommand also calls via closePipes), so the joined
	// error is not an ExitError and code degrades to -1. The contract under
	// test is only about the truncated flag: EOF on both readers means the
	// capture is complete even though a close error exists.
	if result["truncated"] != false {
		t.Fatalf("truncated = %v, want false: EOF was reached on both readers, so the capture is complete even though Close failed", result["truncated"])
	}
	if msg, _ := result["message"].(string); msg != "" && !strings.Contains(msg, closeErr.Error()) {
		t.Fatalf("message = %q, want the close error retained (or empty when nil)", msg)
	}
}
