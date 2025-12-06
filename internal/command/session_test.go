package command

import (
	"bytes"
	"encoding/json"
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

func TestSessionsID_ResolvesFromEnv(t *testing.T) {
	old := os.Getenv("OSM_SESSION_ID")
	defer os.Setenv("OSM_SESSION_ID", old)
	if err := os.Setenv("OSM_SESSION_ID", "env-session"); err != nil {
		t.Fatalf("setenv: %v", err)
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"id"}, &out, &out); err != nil {
		t.Fatalf("id failed: %v", err)
	}
	got := strings.TrimSpace(out.String())
	if got != "env-session" {
		t.Fatalf("expected env-session, got %q", got)
	}
}

func TestSessionsID_RespectsFlag(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	// If the --session flag is provided it should override auto discovery
	var out bytes.Buffer
	if err := cmd.Execute([]string{"id", "--session", "explicit-flag"}, &out, &out); err != nil {
		t.Fatalf("id with flag failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "explicit-flag" {
		t.Fatalf("expected explicit-flag, got %q", out.String())
	}
}

func TestSessionsID_HelpShowsSessionFlag(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"id", "-h"}, &out, &out); err != nil {
		t.Fatalf("id -h failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "-session") {
		t.Fatalf("expected id help to contain -session flag, got: %q", s)
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

func TestSessionsPurge_RemovesAllNonActive(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create two non-active session files
	id1 := "purge-old"
	id2 := "purge-new"
	p1, _ := storage.SessionFilePath(id1)
	p2, _ := storage.SessionFilePath(id2)
	if err := os.WriteFile(p1, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session p1: %v", err)
	}
	if err := os.WriteFile(p2, []byte("{}"), 0644); err != nil {
		t.Fatalf("write session p2: %v", err)
	}

	cfg := config.NewConfig()
	// Set a conservative MaxAge so clean would not remove recent sessions
	cfg.Sessions.MaxAgeDays = 365

	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"purge", "-y"}, &out, &out); err != nil {
		t.Fatalf("purge failed: %v", err)
	}

	if _, err := os.Stat(p1); !os.IsNotExist(err) {
		t.Fatalf("expected session p1 to be removed by purge")
	}
	if _, err := os.Stat(p2); !os.IsNotExist(err) {
		t.Fatalf("expected session p2 to be removed by purge")
	}
}

func TestSessionsPurge_AcceptsDryRunFlag(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"purge", "-dry-run"}, &out, &out); err != nil {
		t.Fatalf("purge -dry-run failed: %v", err)
	}
	if !strings.Contains(out.String(), "Dry-run") {
		t.Fatalf("expected dry-run output for purge, got: %q", out.String())
	}
}

func TestSessionsPurge_HelpShowsFlags(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"purge", "-h"}, &out, &out); err != nil {
		t.Fatalf("purge -h failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "-dry-run") || !strings.Contains(s, "-y") {
		t.Fatalf("expected purge help to mention -dry-run and -y, got: %q", s)
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
	if !strings.Contains(s, "-format") || !strings.Contains(s, "-sort") {
		t.Fatalf("expected help to mention -format and -sort flags, got: %q", s)
	}
}

func TestSessionsClean_HelpShowsFlags(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"clean", "-h"}, &out, &out); err != nil {
		t.Fatalf("clean -h failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "-dry-run") || !strings.Contains(s, "-y") {
		t.Fatalf("expected clean help to mention -dry-run and -y, got: %q", s)
	}
}

func TestSessionsDelete_HelpShowsFlags(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"delete", "-h"}, &out, &out); err != nil {
		t.Fatalf("delete -h failed: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "-dry-run") || !strings.Contains(s, "-y") {
		t.Fatalf("expected delete help to mention -dry-run and -y, got: %q", s)
	}
}

func TestSessionsList_FormatJSON(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	// create two sessions
	s1, _ := storage.SessionFilePath("j1")
	s2, _ := storage.SessionFilePath("j2")
	_ = os.WriteFile(s1, []byte("{}"), 0644)
	_ = os.WriteFile(s2, []byte("{}"), 0644)

	// make j2 active
	l2, _ := storage.SessionLockFilePath("j2")
	_ = os.WriteFile(l2, []byte(""), 0644)

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"list", "-format", "json"}, &out, &out); err != nil {
		t.Fatalf("list json failed: %v", err)
	}
	var got []storage.SessionInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("expected valid json, unmarshal error: %v; output=%q", err, out.String())
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 sessions in json output, got %d: %s", len(got), out.String())
	}
}

func TestSessionsList_FormatJSON_Empty(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"list", "-format", "json"}, &out, &out); err != nil {
		t.Fatalf("list json failed: %v", err)
	}
	var got []storage.SessionInfo
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("expected valid json, unmarshal error: %v; output=%q", err, out.String())
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 sessions in json output, got %d: %s", len(got), out.String())
	}
}

func TestSessionsList_SortActive(t *testing.T) {
	dir := t.TempDir()
	storage.SetTestPaths(dir)

	now := time.Now()
	// three sessions with different times and active status
	ids := []struct {
		id     string
		t      time.Time
		active bool
	}{
		{"a_idle_old", now.Add(-3 * time.Hour), false},
		{"b_active_new", now.Add(-30 * time.Minute), true},
		{"c_idle_newer", now.Add(-10 * time.Minute), false},
	}

	for _, it := range ids {
		p, _ := storage.SessionFilePath(it.id)
		_ = os.WriteFile(p, []byte("{}"), 0644)
		_ = os.Chtimes(p, it.t, it.t)
		if it.active {
			l, _ := storage.SessionLockFilePath(it.id)
			// Acquire and hold an actual lock so ScanSessions reports IsActive=true
			lf, ok, err := storage.AcquireLockHandle(l)
			if err != nil {
				t.Fatalf("failed to acquire lock for %s: %v", it.id, err)
			}
			if !ok {
				t.Fatalf("expected to acquire lock for test setup: %s", it.id)
			}
			// keep lf open until after the command executes
			defer func(f *os.File) { _ = f.Close() }(lf)
		}
	}

	cfg := config.NewConfig()
	cmd := NewSessionCommand(cfg)

	var out bytes.Buffer
	if err := cmd.Execute([]string{"list", "-sort", "active"}, &out, &out); err != nil {
		t.Fatalf("list -sort active failed: %v", err)
	}

	// Split into lines and locate the index of each session id in the output
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")

	idx := map[string]int{"b_active_new": -1, "c_idle_newer": -1, "a_idle_old": -1}
	for i, line := range lines {
		for k := range idx {
			if strings.Contains(line, k) {
				idx[k] = i
			}
		}
	}

	for k, v := range idx {
		if v == -1 {
			t.Fatalf("output missing expected session %q: %s", k, out.String())
		}
	}

	// Expect active session first, then newer idle, then oldest idle
	if !(idx["b_active_new"] < idx["c_idle_newer"] && idx["c_idle_newer"] < idx["a_idle_old"]) {
		t.Fatalf("expected order b_active_new -> c_idle_newer -> a_idle_old, got indices %+v, output=%s", idx, out.String())
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
