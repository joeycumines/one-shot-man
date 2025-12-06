package storage

import (
	"os"
	"path/filepath"
	"time"
)

// SessionInfo holds lightweight metadata for a session file discovered on disk.
type SessionInfo struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	LockPath  string    `json:"lockPath"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
	IsActive  bool      `json:"isActive"`
}

// ScanSessions inspects the configured sessions directory and returns a slice
// of SessionInfo describing each session it finds.
func ScanSessions() ([]SessionInfo, error) {
	dir, err := sessionDirectory()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
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
		if filepath.Ext(name) != ".json" {
			continue
		}

		// Expect name format: {id}.session.json
		base := name[:len(name)-len(".session.json")]
		path := filepath.Join(dir, name)

		fi, err := os.Stat(path)
		if err != nil {
			continue
		}

		lockPath, _ := sessionLockFilePath(base)

		// Try to acquire lock non-blocking. If we succeed, close the
		// descriptor and mark IsActive=false. Crucially, do NOT remove the
		// lock artifact when we can acquire it — removing the file would
		// destroy the lock artifact for sessions that are still valid on
		// disk but currently not held by a process. If we fail to acquire
		// the lock, mark IsActive=true.
		if f, ok, err := AcquireLockHandle(lockPath); err == nil {
			if ok {
				_ = f.Close() // close descriptor but leave artifact
			}

			out = append(out, SessionInfo{
				ID:        base,
				Path:      path,
				LockPath:  lockPath,
				Size:      fi.Size(),
				UpdatedAt: fi.ModTime(),
				CreatedAt: fi.ModTime(),
				IsActive:  !ok,
			})
		} else {
			// On error, still include with IsActive=false so it will be inspected
			out = append(out, SessionInfo{
				ID:        base,
				Path:      path,
				LockPath:  lockPath,
				Size:      fi.Size(),
				UpdatedAt: fi.ModTime(),
				CreatedAt: fi.ModTime(),
				IsActive:  false,
			})
		}
	}

	return out, nil
}
