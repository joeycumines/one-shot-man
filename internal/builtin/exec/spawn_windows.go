//go:build windows

package exec

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// setProcAttr starts the child suspended so it can be assigned to a Job Object
// before its first user instruction runs. Closing the job on every exit path
// makes the process-tree lifetime explicit and prevents descendants from
// escaping containment during the attach/resume window.
func setProcAttr(cmd *osexec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
}

// Windows Job Objects remain valid until the reaper closes them, so there is
// no Unix-style pre-reap process-group observation step.
func waitProcessBeforeReap(*osexec.Cmd) (bool, error) {
	return false, nil
}

type windowsProcessTree struct {
	mu         sync.Mutex
	job        windows.Handle
	ready      chan struct{}
	finished   bool
	attachErr  error
	closed     bool
	terminated bool
}

func newProcessTree() (processTree, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create process job: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		closeErr := windows.CloseHandle(job)
		return nil, errors.Join(fmt.Errorf("configure process job: %w", err), closeErr)
	}
	return &windowsProcessTree{
		job:   job,
		ready: make(chan struct{}),
	}, nil
}

func (t *windowsProcessTree) finish(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.finished {
		return
	}
	t.attachErr = err
	t.finished = true
	close(t.ready)
}

func (t *windowsProcessTree) failAttach(err error) error {
	if err == nil {
		err = os.ErrInvalid
	}
	t.finish(err)
	t.mu.Lock()
	job := t.job
	t.job = 0
	t.closed = true
	t.mu.Unlock()
	if job != 0 {
		if closeErr := windows.CloseHandle(job); closeErr != nil {
			return errors.Join(err, fmt.Errorf("close process job after attach failure: %w", closeErr))
		}
	}
	return err
}

func (t *windowsProcessTree) attach(cmd *osexec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return t.failAttach(os.ErrInvalid)
	}

	t.mu.Lock()
	job := t.job
	closed := t.closed
	t.mu.Unlock()
	if closed || job == 0 {
		return t.failAttach(os.ErrProcessDone)
	}

	var assignErr error
	if err := cmd.Process.WithHandle(func(handle uintptr) {
		assignErr = windows.AssignProcessToJobObject(job, windows.Handle(handle))
	}); err != nil {
		return t.failAttach(fmt.Errorf("open child process handle: %w", err))
	}
	if assignErr != nil {
		return t.failAttach(fmt.Errorf("assign child process to job: %w", assignErr))
	}
	if err := resumeProcess(cmd.Process.Pid); err != nil {
		return t.failAttach(fmt.Errorf("resume child process: %w", err))
	}

	t.finish(nil)
	return nil
}

func resumeProcess(pid int) (err error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("snapshot process threads: %w", err)
	}
	defer func() {
		if closeErr := windows.CloseHandle(snapshot); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close process thread snapshot: %w", closeErr))
		}
	}()

	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		return fmt.Errorf("enumerate process threads: %w", err)
	}
	for {
		if entry.OwnerProcessID == uint32(pid) {
			thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
			if err != nil {
				return fmt.Errorf("open primary process thread: %w", err)
			}
			_, resumeErr := windows.ResumeThread(thread)
			closeErr := windows.CloseHandle(thread)
			if resumeErr != nil {
				return fmt.Errorf("resume primary process thread: %w", resumeErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close primary process thread handle: %w", closeErr)
			}
			return nil
		}
		if err := windows.Thread32Next(snapshot, &entry); err != nil {
			return fmt.Errorf("find primary process thread: %w", err)
		}
	}
}

func (t *windowsProcessTree) kill(_ *osexec.Cmd) error {
	t.mu.Lock()
	ready := t.ready
	t.mu.Unlock()
	<-ready

	t.mu.Lock()
	defer t.mu.Unlock()
	if t.attachErr != nil {
		return t.attachErr
	}
	if t.closed || t.job == 0 {
		return nil
	}
	if t.terminated {
		return nil
	}
	if err := windows.TerminateJobObject(t.job, 1); err != nil {
		return fmt.Errorf("terminate process job: %w", err)
	}
	t.terminated = true
	return nil
}

func (t *windowsProcessTree) close(_ *osexec.Cmd) error {
	// A start failure has no attach phase to signal readiness. Close the gate
	// so CommandContext's cancellation watcher cannot wait forever.
	select {
	case <-t.ready:
	default:
		t.finish(nil)
	}

	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	job := t.job
	t.job = 0
	t.closed = true
	t.mu.Unlock()
	if job == 0 {
		return nil
	}
	return windows.CloseHandle(job)
}

// signalFromName resolves the POSIX-style signal names the JS surface
// accepts. Windows has no POSIX signal delivery: every known name maps to
// os.Interrupt as a dummy accepted os.Signal, and signalProcess provides the
// only (terminate-style) delivery anyway. Unknown names still return nil so a
// typo'd signal rejects with os.ErrInvalid instead of degrading into a kill.
func signalFromName(name string) os.Signal {
	switch name {
	case "SIGHUP", "HUP", "SIGINT", "INT", "SIGQUIT", "QUIT", "SIGTERM", "TERM",
		"SIGUSR1", "USR1", "SIGUSR2", "USR2":
		return os.Interrupt
	default:
		return nil
	}
}

// signalProcess is a Windows approximation: the platform tree owner provides
// terminate-style delivery for the entire process tree.
func signalProcess(cmd *osexec.Cmd, tree processTree, _ os.Signal) error {
	if cmd.Process == nil {
		return nil
	}
	if tree != nil {
		return tree.kill(cmd)
	}
	return cmd.Process.Kill()
}
