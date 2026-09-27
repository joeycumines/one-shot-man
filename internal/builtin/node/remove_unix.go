//go:build unix

package node

import "syscall"

func removeFile(path string, _ bool) error {
	return syscall.Unlink(path)
}

func removeDirectory(path string) error {
	return syscall.Rmdir(path)
}

func normalizeRemoveError(_, _ string, err error) error {
	return err
}
