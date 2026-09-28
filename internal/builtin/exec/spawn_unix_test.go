//go:build unix

package exec

import (
	"errors"
	"os"
	osexec "os/exec"
	"syscall"
	"testing"
)

func TestCloseProcessGroup_ReportsPermissionFailure(t *testing.T) {
	t.Parallel()
	cmd := &osexec.Cmd{Process: &os.Process{Pid: 42}}
	err := closeProcessGroup(cmd, func(pid int, signal syscall.Signal) error {
		if pid != -42 {
			t.Errorf("kill pid = %d, want -42", pid)
		}
		if signal != syscall.SIGKILL {
			t.Errorf("kill signal = %v, want SIGKILL", signal)
		}
		return syscall.EPERM
	})
	if !errors.Is(err, syscall.EPERM) {
		t.Fatalf("closeProcessGroup error = %v, want EPERM", err)
	}
}
