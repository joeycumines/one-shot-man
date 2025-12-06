Applicable as of the commit after `da56e3f6e935ee033a7b6670ce226cf763fe27b5`.

---

This implementation hardens `AcquireLockHandle` against the "stale inode" race condition (where a process acquires a lock on a file descriptor that has been unlinked/deleted by a concurrent cleaner).

It introduces a **verification loop**: after acquiring the lock, it stat-checks the file descriptor against the actual filesystem path to ensure strict identity.

### **File: `internal/storage/filelock.go`**

```go
package storage

import (
	"errors"
	"fmt"
	"os"
)

// ErrWouldBlock signals that a non-blocking lock attempt failed due to the
// resource being locked by another process.
var ErrWouldBlock = errors.New("file lock would block")

// AcquireLockHandle attempts to acquire an exclusive lock on path and returns
// the underlying file handle if successful.
//
// This implementation strictly guarantees that the returned handle corresponds
// to the file currently visible at 'path' in the filesystem. It defends against
// race conditions where a concurrent process (e.g., the cleaner) unlinks the
// file after this process opened it but before it acquired the lock.
func AcquireLockHandle(path string) (*os.File, bool, error) {
	// Loop to handle the rare case where we lock a file that gets deleted
	// effectively 'underneath' us before we verify it.
	for attempt := 0; attempt < 3; attempt++ {
		// 1. Attempt to open and lock the file
		f, err := acquireFileLock(path)
		if err != nil {
			// Ensure no leaks on error
			if f != nil {
				_ = f.Close()
			}
			if errors.Is(err, ErrWouldBlock) {
				return nil, false, nil
			}
			return nil, false, err
		}

		// 2. Verification: Ensure the file we locked is strictly identical to
		// the file currently at 'path'.
		//
		// Race Scenario:
		// T0: We Open(path) -> get FD 3 (pointing to Inode A)
		// T1: Cleaner wakes up, locks Inode A, unlinks 'path', unlocks Inode A.
		// T2: We Flock(FD 3) -> Success (nobody holds lock on Inode A).
		//
		// Without this check, we now hold a lock on a "ghost" file (Inode A)
		// while a new process might create a new 'path' (Inode B).
		fInfo, statErr := f.Stat()
		if statErr != nil {
			_ = f.Close()
			return nil, false, fmt.Errorf("failed to stat locked handle: %w", statErr)
		}

		pInfo, statErr := os.Stat(path)
		if statErr != nil {
			// If path is gone (IsNotExist), the file was deleted after we opened it.
			// We are holding a lock on a stale/deleted file.
			_ = f.Close()
			if os.IsNotExist(statErr) {
				// Loop to recreate/re-acquire on the clean path
				continue
			}
			return nil, false, fmt.Errorf("failed to stat path %q: %w", path, statErr)
		}

		// 3. Identity Check: Compare strictly (Device + Inode on Unix)
		if !os.SameFile(fInfo, pInfo) {
			// The file at 'path' is different from the one we locked.
			// It was likely replaced or recreated concurrently.
			_ = f.Close()
			continue
		}

		// Success: We hold the lock and verified it is authoritative.
		return f, true, nil
	}

	return nil, false, fmt.Errorf("failed to acquire consistent lock after retries")
}

// ReleaseLockHandle releases the lock represented by the provided file handle
// and removes the lock artifact.
func ReleaseLockHandle(f *os.File) error {
	return releaseFileLock(f)
}
```

### Analysis of the Strict Fix

1.  **Race Elimination (`os.SameFile`)**:
    Even with non-blocking locks (`LOCK_NB`), a context switch can occur between `Open` and `Flock`. This fix ensures that `inode(fd) == inode(path)` *after* the lock is acquired. If the inodes differ (or `path` no longer exists), the code detects it holds a stale lock, releases it, and retries.

2.  **Stat Logic**:

      * `f.Stat()`: Queries the metadata of the *open file descriptor*.
      * `os.Stat(path)`: Queries the metadata of the *filesystem entry*.
      * `os.SameFile`: On Unix, this compares `st_dev` and `st_ino`. On Windows, it compares volume serial numbers and file indices. This is portable and strictly correct.

3.  **Retry Loop**:
    The code retries up to 3 times. This is a heuristic to prevent infinite loops in the event of pathological filesystem thrashing, while providing high assurance that a valid lock will eventually be obtained in standard race scenarios.

4.  **Error Handling**:
    If `os.Stat(path)` returns `os.ErrNotExist`, it confirms the cleaner removed the file while we were attempting to lock it. Treating this as a retry condition (which will trigger `O_CREATE` inside `acquireFileLock`) correctly recovers the state.

---

Yes, this implementation is **fully applicable and safe for both Unix-like systems (Linux, macOS) and Windows**, but the *problem* it solves is primarily a Unix-specific phenomenon.

Here is the breakdown of how it applies to each OS architecture:

### 1. Unix / Linux / macOS (The "Inode" Model)
**Applicability: CRITICAL.**

On these systems, a filename (directory entry) is just a pointer to the actual data (inode).
* **The Danger:** You can delete (unlink) a filename while a process has the file open. The process keeps the file descriptor (FD) open to the "deleted" inode.
* **The Race:**
    1.  Your code calls `Open("session.lock")`. You get an FD to **Inode A**.
    2.  *Context Switch*
    3.  The Cleaner runs `os.Remove("session.lock")`. The filesystem unlinks **Inode A**.
    4.  Your code resumes and calls `Flock()` on the FD. **Success.** You now hold a valid lock on a deleted file.
    5.  A new process starts, sees no file, creates **Inode B**, and locks it. **Collision.**
* **The Fix:** `os.SameFile` compares the Inode numbers. It detects that the file you opened (Inode A) is no longer the file in the directory (which is either gone or replaced by Inode B).



### 2. Windows (The "Handle" Model)
**Applicability: Valid, but Redundant (Harmless).**

Windows handles file locking differently. By default, the OS enforces strict sharing rules.
* **The Behavior:** If a process has a file handle open (which happens before you can even attempt the lock), the OS **prevents** other processes from deleting that file.
* **The Scenario:**
    1.  Your code calls `Open("session.lock")`. Windows creates a file handle.
    2.  The Cleaner tries to `os.Remove("session.lock")`.
    3.  **Result:** The Cleaner fails immediately with `Access Denied` (ERROR_SHARING_VIOLATION) because your process holds an open handle.
* **Why the code still works:**
    * Go's `os.SameFile` on Windows is implemented by comparing the Volume Serial Number and File Index (the Windows equivalent of inodes).
    * The check `os.SameFile` will simply pass (return true), because the file couldn't have been deleted in the background anyway.
    * **Benefit:** It keeps your codebase uniform without needing separate `_windows.go` logic.

### Summary
| Feature | Unix/Linux | Windows |
| :--- | :--- | :--- |
| **API Compatibility** | **Yes** (Standard Go `os` lib) | **Yes** (Standard Go `os` lib) |
| **Race Condition** | **Real Risk** (Inodes can be unlinked) | **Blocked by OS** (Open handle prevents delete) |
| **Fix Behavior** | Detects mismatch and retries. | Pass-through (Verification succeeds). |

**Recommendation:** Use the single implementation provided above. It fixes the critical Unix hole while incurring negligible overhead on Windows.
