package storage

import (
	"strings"
	"testing"
)

// ==========================================================================
// InMemoryBackend — json.Marshal error in LoadSession (line 51-53)
// ==========================================================================

func TestInMemoryBackend_LoadSession_MarshalError(t *testing.T) {
	defer ClearAllInMemorySessions()

	b, err := NewInMemoryBackend("marshal-err-load")
	if err != nil {
		t.Fatal(err)
	}

	// Directly inject a session with an unmarshalable field (func) into
	// the global store. json.Marshal will fail on this.
	globalInMemoryStore.Lock()
	globalInMemoryStore.sessions["marshal-err-load"] = &Session{
		ID:          "marshal-err-load",
		Version:     CurrentSchemaVersion,
		SharedState: map[string]any{"bad": func() {}},
	}
	globalInMemoryStore.Unlock()

	_, err = b.LoadSession("marshal-err-load")
	if err == nil {
		t.Fatal("expected error from LoadSession with unmarshalable data")
	}
	if !strings.Contains(err.Error(), "failed to marshal session") {
		t.Errorf("expected 'failed to marshal session' error, got: %v", err)
	}
}

// ==========================================================================
// InMemoryBackend — json.Marshal error in SaveSession (line 71-73)
// ==========================================================================

func TestInMemoryBackend_SaveSession_MarshalError(t *testing.T) {
	defer ClearAllInMemorySessions()

	b, err := NewInMemoryBackend("marshal-err-save")
	if err != nil {
		t.Fatal(err)
	}

	// Create a session with an unmarshalable field.
	session := &Session{
		ID:          "marshal-err-save",
		Version:     CurrentSchemaVersion,
		SharedState: map[string]any{"bad": make(chan int)},
	}

	err = b.SaveSession(session)
	if err == nil {
		t.Fatal("expected error from SaveSession with unmarshalable data")
	}
	if !strings.Contains(err.Error(), "failed to marshal session") {
		t.Errorf("expected 'failed to marshal session' error, got: %v", err)
	}
}

// ==========================================================================
// AtomicWriteFile — MkdirAll error (line 41). Using null byte in path.
// ==========================================================================

func TestAtomicWriteFile_InvalidDirectory(t *testing.T) {
	t.Parallel()
	// Null byte in path causes MkdirAll to fail on all platforms.
	err := AtomicWriteFile("/dev/null\x00/bad/path.json", []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error for invalid directory path")
	}
	if !strings.Contains(err.Error(), "failed to create directory") {
		t.Errorf("expected 'failed to create directory' error, got: %v", err)
	}
}

// ==========================================================================
// AtomicWriteFile — CreateTemp error (line 46). Dir exists but is a file.
// ==========================================================================

func TestAtomicWriteFile_DirIsFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Create a regular file where the dir should be, so CreateTemp fails.
	filePath := dir + "/notadir"
	if err := AtomicWriteFile(filePath, []byte("placeholder"), 0644); err != nil {
		t.Fatal(err)
	}

	// Now try to write UNDER that file as if it were a directory.
	err := AtomicWriteFile(filePath+"/child.json", []byte("data"), 0644)
	if err == nil {
		t.Fatal("expected error when parent is a file, not a directory")
	}
	// Could be either "failed to create directory" or "failed to create temp file"
	// depending on OS behavior with MkdirAll vs CreateTemp.
}
