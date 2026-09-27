package node

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsRemoveErrorsMapToNodeCodes(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "missing file", err: syscall.ERROR_FILE_NOT_FOUND, want: "ENOENT"},
		{name: "missing parent", err: syscall.ERROR_PATH_NOT_FOUND, want: "ENOENT"},
		{name: "non-empty directory", err: syscall.ERROR_DIR_NOT_EMPTY, want: "ENOTEMPTY"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := normalizeRemoveError("rmdir", `C:\parent\child`, test.err)
			if got := errnoCode(err); got != test.want {
				t.Fatalf("errnoCode(%v) = %q, want %q", test.err, got, test.want)
			}
		})
	}
}

func TestWindowsRemoveLongPaths(t *testing.T) {
	dir := windowsLongDirectory(t)
	if err := removeDirectory(dir); err != nil {
		t.Fatalf("remove long directory: %v", err)
	}
	if _, err := os.Lstat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("long directory still exists or returned an unexpected error: %v", err)
	}

	file := filepath.Join(windowsLongDirectory(t), "file")
	if err := os.WriteFile(file, []byte("data"), 0o600); err != nil {
		t.Fatalf("write long-path file: %v", err)
	}
	if err := removeFile(file, false); err != nil {
		t.Fatalf("remove long-path file: %v", err)
	}
	if _, err := os.Lstat(file); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("long-path file still exists or returned an unexpected error: %v", err)
	}
}

func TestWindowsRemoveLongPathWithTrailingPeriod(t *testing.T) {
	path := windowsPathAtLength(t, 252, ".")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("create long-path directory with trailing period: %v", err)
	}
	if err := removeDirectory(path); err != nil {
		t.Fatalf("remove long-path directory with trailing period: %v", err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("long-path directory still exists or returned an unexpected error: %v", err)
	}
}

func TestWindowsUnlinkRejectsDirectoryAfterStaleSymlinkCheck(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "directory")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("create directory: %v", err)
	}
	err := removeFile(dir, true)
	if err == nil {
		t.Fatal("removeFile(symlink=true): want the replacement directory to be rejected")
	}
	if got := errnoCode(err); got != "EACCES" {
		t.Fatalf("errnoCode = %q, want EACCES", got)
	}
	if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
		t.Fatalf("replacement directory was removed: info=%v err=%v", info, err)
	}
}

func TestWindowsRemovalPathPointerMatchesLongPathPolicy(t *testing.T) {
	path := windowsPathAtLength(t, 252, ".")
	for _, test := range []struct {
		name             string
		longPathsEnabled bool
		wantPrefix       bool
	}{
		{name: "long-path-aware", longPathsEnabled: true},
		{name: "legacy", wantPrefix: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			pointer, err := removalPathPointerWithPolicy(path, test.longPathsEnabled)
			if err != nil {
				t.Fatalf("removalPathPointerWithPolicy: %v", err)
			}
			got := windows.UTF16PtrToString(pointer)
			hasPrefix := strings.HasPrefix(got, `\\?\`)
			if hasPrefix != test.wantPrefix {
				t.Fatalf("extended-prefix presence = %t for %q, want %t", hasPrefix, got, test.wantPrefix)
			}
		})
	}
}

func TestWindowsRemovalPathPointerPreservesSpecialPaths(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "extended", path: `\\?\C:\path\name`, want: `\\?\C:\path\name`},
		{name: "device", path: `\\.\pipe\name`, want: `\\.\pipe\name`},
		{name: "nt", path: `\??\C:\path\name`, want: `\??\C:\path\name`},
		{
			name: "long UNC",
			path: `\\server\share\` + strings.Repeat("u", 240),
			want: `\\?\UNC\server\share\` + strings.Repeat("u", 240),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pointer, err := removalPathPointerWithPolicy(test.path, false)
			if err != nil {
				t.Fatalf("removalPathPointerWithPolicy: %v", err)
			}
			if got := windows.UTF16PtrToString(pointer); got != test.want {
				t.Fatalf("removal path = %q, want %q", got, test.want)
			}
		})
	}

	relative := strings.Repeat("r", 248)
	pointer, err := removalPathPointerWithPolicy(relative, false)
	if err != nil {
		t.Fatalf("removalPathPointerWithPolicy(relative): %v", err)
	}
	if got := windows.UTF16PtrToString(pointer); !strings.HasPrefix(got, `\\?\`) {
		t.Fatalf("long relative path = %q, want an extended absolute path", got)
	}
}

func windowsLongDirectory(t *testing.T) string {
	path := windowsPathAtLength(t, 270, "")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("create long-path directory: %v", err)
	}
	return path
}

func windowsPathAtLength(t *testing.T, pathLength int, suffix string) string {
	t.Helper()
	parent := t.TempDir()
	for len(parent)+1+40+len(suffix) < pathLength {
		next := filepath.Join(parent, strings.Repeat("d", 40))
		if err := os.Mkdir(next, 0o700); err != nil {
			t.Fatalf("create long-path directory: %v", err)
		}
		parent = next
	}
	nameLength := pathLength - len(parent) - 1
	if nameLength < len(suffix) {
		t.Fatalf("cannot construct a %d-character path under %q", pathLength, parent)
	}
	return filepath.Join(parent, strings.Repeat("n", nameLength-len(suffix))+suffix)
}
