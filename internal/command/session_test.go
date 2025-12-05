package command

import (
	"bytes"
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting/storage"
)

type errReader struct{}

func (errReader) Read(p []byte) (int, error) { return 0, fmt.Errorf("boom") }

func TestSessionsListAndClean(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create dummy sessions
	// 1. old enough to delete
	// 2. new enough to keep
	now := time.Now()
	old := now.Add(-48 * time.Hour)
	newT := now.Add(-1 * time.Hour)

	s1, _ := storage.SessionFilePath("old_session")
	s2, _ := storage.SessionFilePath("new_session")
	_ = os.WriteFile(s1, []byte("{}"), 0644)
	_ = os.WriteFile(s2, []byte("{}"), 0644)
	_ = os.Chtimes(s1, old, old)
	_ = os.Chtimes(s2, newT, newT)

	// lock file for s2 to simulate active
	l2, _ := storage.SessionLockFilePath("new_session")
	_ = os.WriteFile(l2, []byte(""), 0644)

	cfg := config.NewConfig()
	cfg.Sessions.MaxAgeDays = 1

	cmd := NewSessionCommand(cfg)

	// perform list
	var out bytes.Buffer
	if err := cmd.Execute([]string{"list"}, &out, &out); err != nil {
		t.Fatalf("list failed: %v", err)
	}
	output := out.String()
	if !strings.Contains(output, "old_session") || !strings.Contains(output, "new_session") {
		t.Errorf("expected list to contain both sessions, got: %s", output)
	}

	// perform clean (non-dry) and ensure old sessions removed
	out.Reset()
	if err := cmd.Execute([]string{"clean", "-y"}, &out, &out); err != nil {
		t.Fatalf("clean failed: %v", err)
	}

	if _, err := os.Stat(s1); !os.IsNotExist(err) {
		t.Errorf("expected old_session to be removed")
	}
	if _, err := os.Stat(s2); os.IsNotExist(err) {
		t.Errorf("expected new_session to persist")
	}
}

func TestSessionsPathShowsDirectoryAndFile(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	// without args should print the sessions directory
	if err := cmd.Execute([]string{"path"}, &out, &out); err != nil {
		t.Fatalf("path failed: %v", err)
	}
	outStr := strings.TrimSpace(out.String())
	if outStr == "" || !strings.HasPrefix(outStr, dir) {
		t.Fatalf("expected path output to contain %q, got: %q", dir, outStr)
	}

	// with an id should print file path
	out.Reset()
	id := "my-session"
	expectedFile, _ := storage.SessionFilePath(id)
	if err := cmd.Execute([]string{"path", id}, &out, &out); err != nil {
		t.Fatalf("path id failed: %v", err)
	}
	got := strings.TrimSpace(out.String())
	if got != expectedFile {
		t.Fatalf("expected %q, got %q", expectedFile, got)
	}
}

func TestSessionsDeleteRemovesLockAndFile(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create a session file
	id := "delete-me"
	p, _ := storage.SessionFilePath(id)
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session: %v", err)
	}
	// create a lock file
	lockPath, _ := storage.SessionLockFilePath(id)
	if err := os.WriteFile(lockPath, []byte("pid"), 0644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"delete", "-y", id}, &out, &out); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// session file removed
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("expected session file removed, stat error: %v", err)
	}

	// lock file should not remain
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock file removed, stat error: %v", err)
	}
}

func TestSessionsDeleteAcceptsFlagBeforeID(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create a session file
	id := "delete-after"
	p, _ := storage.SessionFilePath(id)
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"delete", "-y", id}, &out, &out); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// session file removed
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("expected session file removed, stat error: %v", err)
	}

	// lock file should not remain
	lockPath, _ := storage.SessionLockFilePath(id)
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock file removed, stat error: %v", err)
	}
}

func TestSessionsDeleteAcceptsFlagAfterID(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create a session file
	id := "delete-after-flag"
	p, _ := storage.SessionFilePath(id)
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	// Place -y after the id; older flag parsing would treat -y as another id
	if err := cmd.Execute([]string{"delete", id, "-y"}, &out, &out); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// session file removed
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("expected session file removed, stat error: %v", err)
	}
}

func TestSessionsDeleteMultipleIDsDeletesAll(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create two session files
	id1 := "multi-1"
	id2 := "multi-2"
	p1, _ := storage.SessionFilePath(id1)
	p2, _ := storage.SessionFilePath(id2)
	if err := os.WriteFile(p1, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session1: %v", err)
	}
	if err := os.WriteFile(p2, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session2: %v", err)
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"delete", "-y", id1, id2}, &out, &out); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	if _, err := os.Stat(p1); !os.IsNotExist(err) {
		t.Fatalf("expected session file 1 removed, stat error: %v", err)
	}
	if _, err := os.Stat(p2); !os.IsNotExist(err) {
		t.Fatalf("expected session file 2 removed, stat error: %v", err)
	}
}

func TestSessionsClean_AcceptsDryRunFlag(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	// Passing -dry-run after subcommand must be accepted and treated as dry-run
	if err := cmd.Execute([]string{"clean", "-dry-run"}, &out, &out); err != nil {
		t.Fatalf("clean -dry-run failed: %v", err)
	}
	// dry-run should print the dry-run header
	if !strings.Contains(out.String(), "Dry-run") {
		t.Fatalf("expected dry-run output, got: %q", out.String())
	}
}

func TestSessionsDeleteAcceptsDryRunAfterID(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create a session file
	id := "delete-after-dry"
	p, _ := storage.SessionFilePath(id)
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	// Place -dry-run after the id along with -y for non-interactive
	if err := cmd.Execute([]string{"delete", id, "-dry-run", "-y"}, &out, &out); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// session file must remain due to dry-run
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected session file to remain after dry-run, stat error: %v", err)
	}
}

func TestSessionsDeleteAcceptsIDThatLooksLikeFlagWithTerminator(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create a session file with id equal to '-y'
	id := "-y"
	p, _ := storage.SessionFilePath(id)
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	// Provide confirmation before the terminator and then the id after `--`.
	if err := cmd.Execute([]string{"delete", "-y", "--", "-y"}, &out, &out); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	// session file removed
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("expected session file removed, stat error: %v", err)
	}

	// lock file should not remain
	lockPath, _ := storage.SessionLockFilePath(id)
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock file removed, stat error: %v", err)
	}
}

func TestSessionsList_Help(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"list", "-h"}, &out, &out); err != nil {
		t.Fatalf("list -h failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "Usage:") {
		t.Fatalf("expected usage output, got: %q", s)
	}
}

func TestSessionsDelete_NoStdinAborts(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create a session file
	id := "delete-no-stdin"
	p, _ := storage.SessionFilePath(id)
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer

	// Simulate EOF on stdin by injecting an empty reader into the command
	cmd.stdin = strings.NewReader("")

	if err := cmd.Execute([]string{"delete", id}, &out, &out); err != nil {
		t.Fatalf("expected abort without error on EOF, got: %v", err)
	}

	// session file should still exist
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("expected session file to still exist after aborted delete, stat error: %v", err)
	}
}

func TestSessionsDelete_ReadErrorReturnsError(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	id := "delete-read-err"
	p, _ := storage.SessionFilePath(id)
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer

	// Inject a stdin reader that returns an error
	cmd.stdin = errReader{}

	if err := cmd.Execute([]string{"delete", id}, &out, &out); err == nil {
		t.Fatalf("expected read error to be returned, got nil")
	}
}

func TestSessionsDelete_UnknownFlagReturnsError(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	// Pass an unknown flag. It should be caught by fs.Parse inside the manual scanner logic.
	err := cmd.Execute([]string{"delete", "-unknownflag", "session-id"}, &out, &out)

	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}

	if !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("expected undefined flag error, got: %v", err)
	}
}

func TestSessionsDeletePreservesLockOnRemoveFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based remove failure tests skipped on Windows")
	}

	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create a session file
	id := "delete-remove-fail"
	p, _ := storage.SessionFilePath(id)
	if err := os.WriteFile(p, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	// lock file must exist to be preserved
	lockPath, _ := storage.SessionLockFilePath(id)
	if err := os.WriteFile(lockPath, []byte("pid"), 0644); err != nil {
		t.Fatalf("write lock: %v", err)
	}

	// Make the session file unwritable/undeletable by removing permissions on the PARENT directory?
	// Actually, typically standard `os.Remove` fails if the file is open (Windows) or directory permissions (Linux).
	// But `storage.SessionFilePath` returns a path inside `dir`.
	// We can try to make the file immutable or the directory read-only.
	// Simpler approach: chmod the parent dir to 0500 (read+exec, no write).
	parent := dir // storage.SetTestPaths(dir) uses dir directly as base
	if err := os.Chmod(parent, 0500); err != nil {
		t.Fatalf("chmod failed: %v", err)
	}
	defer os.Chmod(parent, 0755)

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	// Attempt delete — remove should fail and the command should return an error
	if err := cmd.Execute([]string{"delete", "-y", id}, &out, &out); err == nil {
		t.Fatalf("expected delete to fail due to remove permission error, got nil")
	}

	// Restore permissions to check state
	_ = os.Chmod(parent, 0755)

	// lock file should still exist because deletion of the session file failed
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatalf("expected lock file to remain after failed deletion")
	}
}
