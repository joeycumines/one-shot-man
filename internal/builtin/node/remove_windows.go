package node

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func removeFile(path string, symlink bool) error {
	name, err := removalPathPointer(path)
	if err != nil {
		return err
	}
	if symlink {
		err = removeSymlink(name)
	} else {
		err = syscall.DeleteFile(name)
	}
	return normalizeRemoveError("unlink", path, err)
}

func removeSymlink(name *uint16) (err error) {
	// The reparse-point flag keeps the handle bound to the link, not its target.
	handle, err := windows.CreateFile(
		name,
		windows.DELETE|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, windows.CloseHandle(handle))
	}()

	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT == 0 {
		return syscall.ERROR_ACCESS_DENIED
	}

	disposition := struct{ DeleteFile byte }{DeleteFile: 1}
	return windows.SetFileInformationByHandle(
		handle,
		windows.FileDispositionInfo,
		(*byte)(unsafe.Pointer(&disposition)),
		uint32(unsafe.Sizeof(disposition)),
	)
}

func removeDirectory(path string) error {
	name, err := removalPathPointer(path)
	if err != nil {
		return err
	}
	return normalizeRemoveError("rmdir", path, syscall.RemoveDirectory(name))
}

func removalPathPointer(path string) (*uint16, error) {
	return removalPathPointerWithPolicy(path, windowsCanUseLongPaths())
}

func removalPathPointerWithPolicy(path string, longPathsEnabled bool) (*uint16, error) {
	if path == "" || longPathsEnabled || hasExtendedWindowsPrefix(path) {
		return syscall.UTF16PtrFromString(path)
	}

	pathLength := len(path)
	if !filepath.IsAbs(path) {
		workingDirectory, err := os.Getwd()
		if err == nil {
			pathLength += len(workingDirectory)
		}
		pathLength++
	}
	if pathLength < 248 {
		return syscall.UTF16PtrFromString(path)
	}

	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if hasExtendedWindowsPrefix(absolute) {
		return syscall.UTF16PtrFromString(absolute)
	}
	// Match os.fixLongPath's threshold while keeping the type-specific syscall.
	if strings.HasPrefix(absolute, `\\`) {
		absolute = `\\?\UNC\` + strings.TrimLeft(absolute, `\/`)
	} else {
		absolute = `\\?\` + absolute
	}
	return syscall.UTF16PtrFromString(absolute)
}

func windowsCanUseLongPaths() bool {
	// Match runtime.initLongPathSupport, which enables the same process flag
	// that os.fixLongPath checks.
	version := windows.RtlGetVersion()
	if version.MajorVersion < 10 {
		return false
	}
	return version.MajorVersion > 10 || version.MinorVersion != 0 || version.BuildNumber >= 15063
}

func hasExtendedWindowsPrefix(path string) bool {
	if len(path) < 4 {
		return false
	}
	isSeparator := func(b byte) bool { return b == '\\' || b == '/' }
	if path[:4] == `\??\` {
		return true
	}
	return isSeparator(path[0]) && isSeparator(path[1]) &&
		(path[2] == '?' || path[2] == '.') && isSeparator(path[3])
}

func normalizeRemoveError(op, path string, err error) error {
	errno, ok := errors.AsType[syscall.Errno](err)
	if !ok {
		return err
	}
	switch errno {
	case syscall.ERROR_PATH_NOT_FOUND:
		errno = syscall.ENOENT
	case syscall.ERROR_DIR_NOT_EMPTY:
		errno = syscall.ENOTEMPTY
	default:
		return err
	}
	return &os.PathError{Op: op, Path: path, Err: errno}
}
