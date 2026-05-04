package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionInfo holds lightweight metadata for a session file discovered on disk.
type SessionInfo struct {
	ID         string    `json:"id"`
	Path       string    `json:"path"`
	LockPath   string    `json:"lockPath"`
	Size       int64     `json:"size"`
	UpdateTime time.Time `json:"updateTime"`
	CreateTime time.Time `json:"createTime"`
	Active     bool      `json:"active"`
}

// ScanSessions inspects the configured sessions directory and returns a slice
// of SessionInfo describing each session it finds.
func ScanSessions() ([]SessionInfo, error) {
	dir, err := getSessionDirectory()
	if err != nil {
		return nil, err
	}

	// On some platforms (notably Windows), os.ReadDir on a non-directory path
	// may not return an error. Explicitly verify the path is a directory.
	if info, statErr := os.Stat(dir); statErr == nil && !info.IsDir() {
		return nil, fmt.Errorf("sessions path is not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []SessionInfo{}, nil
		}
		return nil, err
	}

	var out []SessionInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		const suffix = ".session.json"
		if !strings.HasSuffix(name, suffix) {
			continue
		}

		base := name[:len(name)-len(suffix)]
		path := filepath.Join(dir, name)

		fi, err := os.Stat(path)
		if err != nil {
			continue
		}

		lockPath, _ := getSessionLockFilePath(base)

		// Try to acquire lock non-blocking. If we succeed, close the
		// descriptor and mark Active=false. Crucially, do NOT remove the
		// lock artifact when we can acquire it — removing the file would
		// destroy the lock artifact for sessions that are still valid on
		// disk but currently not held by a process. If we fail to acquire
		// the lock, mark Active=true.
		if f, ok, err := AcquireLockHandle(lockPath); err == nil {
			if ok {
				_ = f.Close() // close descriptor but leave artifact
			}

			out = append(out, SessionInfo{
				ID:         base,
				Path:       path,
				LockPath:   lockPath,
				Size:       fi.Size(),
				UpdateTime: fi.ModTime(),
				CreateTime: fi.ModTime(),
				Active:     !ok,
			})
		} else {
			// On error, still include with Active=false so it will be inspected
			out = append(out, SessionInfo{
				ID:         base,
				Path:       path,
				LockPath:   lockPath,
				Size:       fi.Size(),
				UpdateTime: fi.ModTime(),
				CreateTime: fi.ModTime(),
				Active:     false,
			})
		}
	}

	return out, nil
}
