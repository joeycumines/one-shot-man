package exec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type captureDeadlineReader struct {
	io.ReadCloser
	closeErr   error
	closeCalls atomic.Int32
}

func (r *captureDeadlineReader) Close() error {
	r.closeCalls.Add(1)
	return errors.Join(r.ReadCloser.Close(), r.closeErr)
}

// completedDeadlineOutput obtains real EOF records from native pipes before
// the test enters the collector's expired-grace phase. Requeueing these exact
// records fixes the ordering without relying on select's random timer choice.
func completedDeadlineOutput(t *testing.T, closeErr error) ([2]*captureDeadlineReader, []commandOutputResult) {
	t.Helper()
	var readers [2]*captureDeadlineReader
	for i, payload := range []string{"complete stdout\n", "complete stderr\n"} {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = r.Close()
			_ = w.Close()
		})
		readers[i] = &captureDeadlineReader{ReadCloser: r}
		if i == 0 {
			readers[i].closeErr = closeErr
		}
		if n, err := w.WriteString(payload); err != nil || n != len(payload) {
			t.Fatalf("write pipe %d: bytes=%d error=%v", i, n, err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	results := startCommandOutputReaders(readers[0], readers[1])
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	completed := make([]commandOutputResult, 0, 2)
	for len(completed) < 2 {
		select {
		case result := <-results:
			if !result.eof {
				t.Fatalf("native pipe did not reach EOF: %+v", result)
			}
			completed = append(completed, result)
		case <-timer.C:
			t.Fatal("native pipe readers did not publish completion")
		}
	}
	return readers, completed
}

func TestCommandOutputCapture_DeadlineOrdering(t *testing.T) {
	for _, closeFailure := range []bool{false, true} {
		for _, alreadyStored := range []int{0, 1} {
			t.Run(fmt.Sprintf("published_eof/close_failure_%v/stored_%d", closeFailure, alreadyStored), func(t *testing.T) {
				var closeErr error
				if closeFailure {
					closeErr = errors.New("post-EOF close failure")
				}
				readers, records := completedDeadlineOutput(t, closeErr)
				ready := make(chan commandOutputResult, 2)
				collector := commandOutputCollector{results: ready}
				for i, record := range records {
					if i < alreadyStored {
						collector.store(record, false)
					} else {
						ready <- record
					}
				}
				// Enter the actual phase selected by the first grace timer.
				stdout, stderr, truncated, err := collector.forceClose(context.Canceled, readers[0], readers[1])
				if stdout != "complete stdout\n" || stderr != "complete stderr\n" || truncated {
					t.Fatalf("published EOF lost at grace deadline: stdout=%q stderr=%q truncated=%v error=%v", stdout, stderr, truncated, err)
				}
				if closeErr == nil && err != nil || closeErr != nil && !errors.Is(err, closeErr) {
					t.Fatalf("recorded close result changed: got %v want %v", err, closeErr)
				}
				if _, aborted := errors.AsType[*outputCaptureAbortedError](err); aborted {
					t.Fatalf("completed output gained a capture-abort error: %v", err)
				}
				for i, reader := range readers {
					if calls := reader.closeCalls.Load(); calls != 1 {
						t.Errorf("completed pipe %d was forcibly closed again: calls=%d", i, calls)
					}
				}
			})
		}
	}

	t.Run("unpublished_close_induced_eof_stays_truncated", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			stdoutPipe := &eofOnClosePipe{closed: make(chan struct{})}
			stderrPipe := &eofReadCloser{}
			collector := commandOutputCollector{results: startCommandOutputReaders(stdoutPipe, stderrPipe)}
			synctest.Wait()
			_, _, truncated, err := collector.forceClose(context.Canceled, stdoutPipe, stderrPipe)
			if !truncated || !errors.Is(err, context.Canceled) || collector.received != 2 {
				t.Fatalf("forced Close became complete: truncated=%v error=%v received=%d", truncated, err, collector.received)
			}
			if _, aborted := errors.AsType[*outputCaptureAbortedError](err); !aborted {
				t.Fatalf("forced Close lost the capture-abort diagnostic: %v", err)
			}
		})
	})

	for _, alreadyStored := range []int{0, 1} {
		t.Run(fmt.Sprintf("final_deadline_retains_published_prefix/stored_%d", alreadyStored), func(t *testing.T) {
			ready := make(chan commandOutputResult, 2)
			collector := commandOutputCollector{results: ready}
			for i, result := range []commandOutputResult{
				{index: 0, data: []byte("retained stdout prefix\n"), err: os.ErrClosed},
				{index: 1, data: []byte("complete stderr\n"), eof: true},
			} {
				if i < alreadyStored {
					collector.store(result, true)
				} else {
					ready <- result
				}
			}
			// Enter the actual return path selected by the second grace timer.
			stdout, stderr, truncated, err := collector.aborted(context.Canceled)
			if stdout != "retained stdout prefix\n" || stderr != "complete stderr\n" || !truncated {
				t.Fatalf("published prefix lost at final deadline: stdout=%q stderr=%q truncated=%v", stdout, stderr, truncated)
			}
			if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), os.ErrClosed.Error()) {
				t.Fatalf("capture-abort cause changed or gained induced-close noise: %v", err)
			}
		})
	}
}

// captureFirstCloseFailure reports a fault only on its first Close. The
// interrupted reader subsequently sees ordinary closed-file errors, so only
// the collector that forced closure can preserve the original failure.
type captureFirstCloseFailure struct {
	closed chan struct{}
	calls  atomic.Int32
	err    error
}

func (r *captureFirstCloseFailure) Read([]byte) (int, error) {
	<-r.closed
	return 0, os.ErrClosed
}

func (r *captureFirstCloseFailure) Close() error {
	if r.calls.Add(1) == 1 {
		close(r.closed)
		return r.err
	}
	return os.ErrClosed
}

func TestCommandOutputCapture_ErrorFacts(t *testing.T) {
	independent := errors.New("independent output failure")
	opaque := fmt.Errorf("opaque close diagnostic: %w", error(nil))
	for _, tc := range []struct {
		name        string
		err         error
		wantCause   error
		wantWrapper bool
	}{
		{name: "closed_only", err: os.ErrClosed},
		{name: "wrapped_closed_only", err: fmt.Errorf("closed wrapper: %w", os.ErrClosed)},
		{name: "joined_independent", err: errors.Join(os.ErrClosed, independent), wantCause: independent},
		{name: "wrapped_mixed_join", err: fmt.Errorf("outer output diagnostic: %w", errors.Join(os.ErrClosed, independent)), wantCause: independent, wantWrapper: true},
		{name: "nil_unwrap_failure", err: opaque, wantCause: opaque},
	} {
		t.Run("close_result/"+tc.name, func(t *testing.T) {
			readers, records := completedDeadlineOutput(t, tc.err)
			ready := make(chan commandOutputResult, 2)
			for _, record := range records {
				ready <- record
			}
			collector := commandOutputCollector{results: ready}
			stdout, stderr, truncated, err := collector.forceClose(context.Canceled, readers[0], readers[1])
			if stdout != "complete stdout\n" || stderr != "complete stderr\n" || truncated {
				t.Fatalf("close diagnostic changed captured EOF: stdout=%q stderr=%q truncated=%v", stdout, stderr, truncated)
			}
			if tc.wantCause != nil && !errors.Is(err, tc.wantCause) {
				t.Fatalf("independent close failure was erased: %v", err)
			}
			if tc.wantCause == nil && err != nil {
				t.Fatalf("benign closed-only result became a failure: %v", err)
			}
			if tc.wantWrapper && !strings.Contains(err.Error(), "outer output diagnostic") {
				t.Fatalf("independent close wrapper was erased: %v", err)
			}
		})
		t.Run("forced_result/"+tc.name, func(t *testing.T) {
			ready := make(chan commandOutputResult, 2)
			ready <- commandOutputResult{index: 0, data: []byte("retained stdout prefix\n"), err: tc.err}
			ready <- commandOutputResult{index: 1, eof: true}
			collector := commandOutputCollector{results: ready}
			stdout, _, truncated, err := collector.aborted(context.Canceled)
			if stdout != "retained stdout prefix\n" || !truncated || !errors.Is(err, context.Canceled) {
				t.Fatalf("forced capture facts changed: stdout=%q truncated=%v error=%v", stdout, truncated, err)
			}
			if tc.wantCause != nil && !errors.Is(err, tc.wantCause) {
				t.Fatalf("independent forced-capture failure was erased: %v", err)
			}
			if tc.wantCause == nil && errors.Is(err, os.ErrClosed) {
				t.Fatalf("benign induced-close noise was retained: %v", err)
			}
			if tc.wantWrapper && !strings.Contains(err.Error(), "outer output diagnostic") {
				t.Fatalf("independent forced-capture wrapper was erased: %v", err)
			}
		})
	}

	for _, stream := range []string{"stdout", "stderr"} {
		t.Run("first_forced_close/"+stream, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				fault := errors.New("first forced-close failure")
				failedPipe := &captureFirstCloseFailure{closed: make(chan struct{}), err: errors.Join(os.ErrClosed, fault)}
				var stdoutPipe, stderrPipe io.ReadCloser = &eofReadCloser{}, &eofReadCloser{}
				if stream == "stdout" {
					stdoutPipe = failedPipe
				} else {
					stderrPipe = failedPipe
				}
				collector := commandOutputCollector{results: startCommandOutputReaders(stdoutPipe, stderrPipe)}
				synctest.Wait()
				_, _, truncated, err := collector.forceClose(context.Canceled, stdoutPipe, stderrPipe)
				if !truncated || !errors.Is(err, context.Canceled) || collector.received != 2 {
					t.Fatalf("forced close changed capture facts: truncated=%v error=%v received=%d", truncated, err, collector.received)
				}
				if !errors.Is(err, fault) || !strings.Contains(err.Error(), "close command "+stream+" pipe") {
					t.Fatalf("first forced-close failure was erased: stream=%s error=%v", stream, err)
				}
			})
		})
	}

	for _, tc := range []struct {
		name          string
		err           error
		wantTruncated bool
	}{
		{name: "exact_eof", err: io.EOF},
		{name: "joined_eof_failure", err: errors.Join(io.EOF, independent), wantTruncated: true},
		{name: "wrapped_joined_eof_failure", err: fmt.Errorf("read wrapper: %w", errors.Join(io.EOF, independent)), wantTruncated: true},
	} {
		t.Run("read_result/"+tc.name, func(t *testing.T) {
			stdoutPipe := &prefixThenErrReader{payload: []byte("read prefix\n"), cause: tc.err}
			stderrPipe := io.NopCloser(strings.NewReader("complete stderr\n"))
			ready := make(chan commandOutputResult, 2)
			readCommandOutput(0, stdoutPipe, ready)
			readCommandOutput(1, stderrPipe, ready)
			collector := commandOutputCollector{results: ready}
			stdout, stderr, truncated, err := collector.forceClose(context.Canceled, stdoutPipe, stderrPipe)
			if truncated != tc.wantTruncated {
				t.Fatalf("ReadAll failure became complete: truncated=%v want=%v error=%v", truncated, tc.wantTruncated, err)
			}
			if stdout != "read prefix\n" || stderr != "complete stderr\n" {
				t.Fatalf("read result lost captured bytes: stdout=%q stderr=%q", stdout, stderr)
			}
			if tc.wantTruncated && !errors.Is(err, independent) || !tc.wantTruncated && err != nil {
				t.Fatalf("read failure diagnostic changed: %v", err)
			}
		})
	}
}
