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
	result := runExecCommand(ctx, command, args, pipeFactory, newProcessTree, outputEOFCeiling)

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
	result := runExecCommand(ctx, command, args, pipeFactory, newProcessTree, outputEOFCeiling)

	if result["code"] != 0 {
		t.Fatalf("code = %v, want 0: the child exited 0 and ProcessState holds the real status even though Close failed after EOF", result["code"])
	}
	if result["truncated"] != false {
		t.Fatalf("truncated = %v, want false: EOF was reached on both readers, so the capture is complete even though Close failed", result["truncated"])
	}
	if msg, _ := result["message"].(string); msg != "" && !strings.Contains(msg, closeErr.Error()) {
		t.Fatalf("message = %q, want the close error retained (or empty when nil)", msg)
	}
}

// cancelOnEOFReader wraps a real pipe reader and triggers cancel strictly after
// the child closes its write end (EOF), guaranteeing the child exited cleanly
// before ctx.Err() becomes non-nil.
type cancelOnEOFReader struct {
	commandPipeEndpoint
	cancel context.CancelFunc
}

func (r *cancelOnEOFReader) Read(p []byte) (int, error) {
	n, err := r.commandPipeEndpoint.Read(p)
	if errors.Is(err, io.EOF) && r.cancel != nil {
		r.cancel()
	}
	return n, err
}

// TestRunExec_CancelAfterCleanExitPreservesCode pins review-16 Blocker 1: a
// child that exits 0 with a complete capture (EOF on both streams) must
// preserve code 0 and error:false even if ctx cancels concurrently with or
// after the reap. Unconditionally joining ctx.Err() into waitErr and
// inferring code from error shape turned a passing command into {code:-1,
// error:true}.
func TestRunExec_CancelAfterCleanExitPreservesCode(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	pipeFactory := func() (commandPipeEndpoint, commandPipeEndpoint, error) {
		calls++
		r, w, err := newCommandPipe()
		if err != nil {
			return nil, nil, err
		}
		if calls == 1 {
			return &cancelOnEOFReader{commandPipeEndpoint: r, cancel: cancel}, w, nil
		}
		return r, w, nil
	}

	command, args := exitCodeCommand(0)
	result := runExecCommand(ctx, command, args, pipeFactory, newProcessTree, outputEOFCeiling)

	if result["code"] != 0 {
		t.Fatalf("code = %v, want 0 (message=%q)", result["code"], result["message"])
	}
	if result["truncated"] != false {
		t.Fatalf("truncated = %v, want false", result["truncated"])
	}
	if result["error"] != false {
		t.Fatalf("error = %v, want false for a clean exit 0 with complete capture (message=%q)", result["error"], result["message"])
	}
}
