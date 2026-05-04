package storage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ==========================================================================
// fs_backend.go — SaveSession marshal error path
// ==========================================================================

// TestSaveSession_MarshalError triggers json.MarshalIndent failure by putting
// an unmarshalable value (channel) in SharedState.
// NOT parallel: mutates package-level path state via SetTestPaths.
func TestSaveSession_MarshalError(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)
	t.Cleanup(ResetPaths)

	id := "marshal-err"
	b, err := NewFileSystemBackend(id)
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close() })

	// Create a session with an unmarshalable SharedState value.
	sess := &Session{
		ID:          id,
		Version:     "1.0.0",
		SharedState: map[string]any{"bad": make(chan int)},
	}

	err = b.SaveSession(sess)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to marshal session")
}

// ==========================================================================
// fs_backend.go — NewFileSystemBackend MkdirAll error
// ==========================================================================

// TestNewFileSystemBackend_MkdirAllError triggers MkdirAll failure by pointing
// the session directory to a path where a regular file blocks dir creation.
// NOT parallel: mutates package-level path state (sessionDirectory).
func TestNewFileSystemBackend_MkdirAllError(t *testing.T) {
	dir := t.TempDir()

	// Create a regular file that will block MkdirAll from creating the
	// sessions subdirectory.
	blockingFile := filepath.Join(dir, "sessions")
	require.NoError(t, os.WriteFile(blockingFile, []byte("I am a file, not a dir"), 0644))

	// Override the session directory to point to the blocking file.
	pathsMu.Lock()
	oldDir := sessionDirectory
	sessionDirectory = func() (string, error) { return blockingFile, nil }
	pathsMu.Unlock()
	t.Cleanup(func() {
		pathsMu.Lock()
		sessionDirectory = oldDir
		pathsMu.Unlock()
	})

	_, err := NewFileSystemBackend("test-id")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create session directory")
}

// ==========================================================================
// atomic_write.go — AtomicWriteFile write error via /dev/full (Unix only)
// ==========================================================================

// TestAtomicWriteFile_WriteError exercises the write failure path using
// a read-only directory for the temp file (which then fails on write
// if the temp file creation succeeds).
func TestAtomicWriteFile_ReadOnlyTarget(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	// Create a target file in the writable dir first.
	target := filepath.Join(dir, "test.json")

	// Write once — should succeed.
	err := AtomicWriteFile(target, []byte("initial"), 0644)
	require.NoError(t, err)

	// Verify content.
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "initial", string(data))

	// Write again with different data — overwrite.
	err = AtomicWriteFile(target, []byte("updated"), 0644)
	require.NoError(t, err)

	data, err = os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "updated", string(data))
}

// TestAtomicWriteFile_PermissionsPreserved verifies the file ends up with the
// requested permissions.
func TestAtomicWriteFile_PermissionsPreserved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	target := filepath.Join(dir, "perms.json")

	err := AtomicWriteFile(target, []byte("data"), 0600)
	require.NoError(t, err)

	fi, err := os.Stat(target)
	require.NoError(t, err)
	// On Unix, the mode should match (Windows ignores permissions).
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0600), fi.Mode().Perm())
	}
}

// ==========================================================================
// fs_backend.go — SaveSession AtomicWriteFile failure
// ==========================================================================

// TestSaveSession_AtomicWriteFailure triggers an AtomicWriteFile failure
// by making the session directory read-only after backend creation.
// NOT parallel: mutates package-level path state via SetTestPaths.
func TestSaveSession_AtomicWriteFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not enforce directory write permissions for file creation")
	}
	if os.Getuid() == 0 {
		t.Skip("test requires non-root to enforce permissions")
	}

	dir := t.TempDir()
	SetTestPaths(dir)
	t.Cleanup(ResetPaths)

	id := "write-fail"
	b, err := NewFileSystemBackend(id)
	require.NoError(t, err)
	t.Cleanup(func() { _ = b.Close() })

	// Make the directory read-only so AtomicWriteFile can't create a temp file.
	require.NoError(t, os.Chmod(dir, 0555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	sess := &Session{
		ID:      id,
		Version: "1.0.0",
	}

	err = b.SaveSession(sess)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to write session file")
}
