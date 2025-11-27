package storage

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

// Verify AcquireLockHandle closes a non-nil *os.File returned alongside an
// error from acquireFileLock so callers cannot leak file descriptors.
func TestAcquireLockHandle_ClosesOnError(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/lock"

	// Create a file we'll return from the stub; keep a pointer so we can
	// validate it is closed by AcquireLockHandle.
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	// Save and restore original implementation
	orig := acquireFileLock
	defer func() { acquireFileLock = orig }()

	// Stub that returns a non-nil file AND an error.
	acquireFileLock = func(p string) (*os.File, error) {
		if p != path {
			return nil, fmt.Errorf("unexpected path: %s", p)
		}
		return f, errors.New("boom")
	}

	gotF, ok, err := AcquireLockHandle(path)
	if gotF != nil {
		_ = gotF.Close()
		t.Fatalf("expected nil file on error")
	}
	if ok {
		t.Fatalf("expected ok=false on error")
	}
	if err == nil {
		t.Fatalf("expected non-nil error")
	}

	// The file returned by the stub should have been closed by AcquireLockHandle.
	if _, werr := f.Write([]byte("x")); werr == nil {
		t.Fatalf("expected file to be closed, write succeeded")
	}
}

// When acquireFileLock returns ErrWouldBlock along with a non-nil file, the
// implementation must still close the file before returning to avoid leaks.
func TestAcquireLockHandle_ClosesOnWouldBlock(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/lock2"

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}

	orig := acquireFileLock
	defer func() { acquireFileLock = orig }()

	acquireFileLock = func(p string) (*os.File, error) {
		return f, ErrWouldBlock
	}

	gotF, ok, err := AcquireLockHandle(path)
	if err != nil {
		t.Fatalf("expected no error for ErrWouldBlock fold, got: %v", err)
	}
	if gotF != nil {
		_ = gotF.Close()
		t.Fatalf("expected nil file when not acquired")
	}
	if ok {
		t.Fatalf("expected ok=false when lock would block")
	}

	if _, werr := f.Write([]byte("x")); werr == nil {
		t.Fatalf("expected stubbed file to be closed on ErrWouldBlock")
	}
}
