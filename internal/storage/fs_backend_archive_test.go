package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArchiveSession_DestAlreadyExists_DoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	SetTestPaths(dir)

	sid := "sid-atomic-check"

	// Create real backend and a session file.
	b, err := NewFileSystemBackend(sid)
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer b.Close()

	// Save a simple session so there's content to archive.
	sess := &Session{ID: sid, Version: currentSchemaVersion, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := b.SaveSession(sess); err != nil {
		t.Fatalf("failed to save session: %v", err)
	}

	// Create a destination file that already exists with distinct contents.
	archivePath, err := ArchiveSessionFilePath(sid, time.Now(), 0)
	if err != nil {
		t.Fatalf("ArchiveSessionFilePath failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(archivePath), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(archivePath, []byte("PREEXISTING"), 0644); err != nil {
		t.Fatalf("failed to create preexisting dest: %v", err)
	}

	// Attempt archive should return os.ErrExist and must not overwrite the preexisting file.
	if err := b.ArchiveSession(sid, archivePath); err == nil {
		t.Fatalf("expected ArchiveSession to return an error when dest exists")
	} else if err != os.ErrExist {
		t.Fatalf("expected os.ErrExist, got: %v", err)
	}

	// Ensure preexisting file retains original contents.
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed to read archive file: %v", err)
	}
	if string(data) != "PREEXISTING" {
		t.Fatalf("archive destination was modified; want PREEXISTING, got: %q", string(data))
	}

	// Session file should still exist and be loadable.
	sessionPath, err := SessionFilePath(sid)
	if err != nil {
		t.Fatalf("SessionFilePath failed: %v", err)
	}
	if _, err := os.Stat(sessionPath); err != nil {
		t.Fatalf("expected session file to still exist after failed archive: %v", err)
	}
}
