//go:build windows

package pty

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// windowsProcessHandle wraps a Windows process handle created via
// CreateProcess with a ConPTY pseudoconsole attached.
type windowsProcessHandle struct {
	process   windows.Handle
	pid       uint32
	conPTY    windows.Handle // pseudoconsole handle; zero when closed
	inputRead windows.Handle // PTY-side input read handle; kept open for ConPTY's reader thread
	mu        sync.Mutex     // guards conPTY against concurrent close/Signal
}

// closeConPTY closes the pseudoconsole handle if not already closed.
// Thread-safe — callers do NOT need to hold any external lock.
func (h *windowsProcessHandle) closeConPTY() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conPTY != 0 {
		windows.ClosePseudoConsole(h.conPTY)
		h.conPTY = 0
	}
	if h.inputRead != 0 {
		windows.CloseHandle(h.inputRead)
		h.inputRead = 0
	}
}

func (h *windowsProcessHandle) Wait() error {
	event, err := windows.WaitForSingleObject(h.process, windows.INFINITE)
	if err != nil {
		return fmt.Errorf("pty: WaitForSingleObject: %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("pty: WaitForSingleObject: unexpected event %d", event)
	}
	return nil
}

func (h *windowsProcessHandle) Signal(sig os.Signal) error {
	switch sig {
	case syscall.SIGTERM:
		// Graceful shutdown: close the pseudoconsole, which signals
		// a console-close event to the child process. If the ConPTY
		// was already closed (e.g. by the process-exit watcher),
		// escalate to terminate.
		h.mu.Lock()
		alreadyClosed := h.conPTY == 0
		h.mu.Unlock()
		h.closeConPTY()
		if alreadyClosed {
			return windows.TerminateProcess(h.process, 1)
		}
		return nil
	default:
		// SIGKILL, SIGINT, SIGHUP, etc. — force terminate.
		return windows.TerminateProcess(h.process, 1)
	}
}

func (h *windowsProcessHandle) Pid() int {
	return int(h.pid)
}

// Spawn allocates a ConPTY pseudoconsole and starts the given command.
// The returned Process must be closed to prevent resource leaks.
//
// On Windows, the pseudoconsole is created with two anonymous pipes:
// one for writing input to the child, one for reading output from the
// child. Process.ptyFile is the output read end; Process.writeFile is
// the input write end.
func Spawn(ctx context.Context, cfg SpawnConfig) (*Process, error) {
	if cfg.Command == "" {
		return nil, errors.New("pty: command is required")
	}
	cfg.applyDefaults()

	// Create two anonymous pipes for ConPTY I/O.
	// Input pipe: parent writes → inputWriteHandle, child reads ← inputReadHandle.
	// Output pipe: child writes → outputWriteHandle, parent reads ← outputReadHandle.
	var inputReadHandle, inputWriteHandle windows.Handle
	var outputReadHandle, outputWriteHandle windows.Handle

	if err := windows.CreatePipe(&inputReadHandle, &inputWriteHandle, nil, 0); err != nil {
		return nil, fmt.Errorf("pty: CreatePipe (input): %w", err)
	}
	if err := windows.CreatePipe(&outputReadHandle, &outputWriteHandle, nil, 0); err != nil {
		windows.CloseHandle(inputReadHandle)
		windows.CloseHandle(inputWriteHandle)
		return nil, fmt.Errorf("pty: CreatePipe (output): %w", err)
	}

	// Create the pseudoconsole. ConPTY reads from inputReadHandle and
	// writes to outputWriteHandle.
	size := windows.Coord{X: int16(cfg.Cols), Y: int16(cfg.Rows)}
	var pconsole windows.Handle
	if err := windows.CreatePseudoConsole(size, inputReadHandle, outputWriteHandle, 0, &pconsole); err != nil {
		windows.CloseHandle(inputReadHandle)
		windows.CloseHandle(inputWriteHandle)
		windows.CloseHandle(outputReadHandle)
		windows.CloseHandle(outputWriteHandle)
		return nil, fmt.Errorf("pty: CreatePseudoConsole: %w", err)
	}

	// Close the PTY-end output handle — the ConPTY has its own copy.
	// Keep inputReadHandle open for the ConPTY's input reader thread.
	windows.CloseHandle(outputWriteHandle)

	proc, err := spawnWithConPTY(ctx, cfg, pconsole, inputWriteHandle, outputReadHandle, inputReadHandle)
	if err != nil {
		windows.ClosePseudoConsole(pconsole)
		windows.CloseHandle(inputReadHandle)
		windows.CloseHandle(inputWriteHandle)
		windows.CloseHandle(outputReadHandle)
		return nil, err
	}
	return proc, nil
}

// spawnWithConPTY creates the child process attached to the given ConPTY.
// On success, ownership of pconsole, inputWrite, and outputRead is
// transferred to the returned Process. The inputRead handle is retained
// (stored on the process handle) and closed later by closeConPTY() when the
// pseudoconsole is torn down; keeping it open for the ConPTY's lifetime avoids
// prematurely severing the input pipe (see the retention note in Spawn above).
func spawnWithConPTY(ctx context.Context, cfg SpawnConfig, pconsole, inputWrite, outputRead, inputRead windows.Handle) (*Process, error) {
	// Set up the process thread attribute list with the pseudoconsole.
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, fmt.Errorf("pty: NewProcThreadAttributeList: %w", err)
	}
	defer attrList.Delete()

	if err := attrList.Update(
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE,
		// Pass the HPCON handle value as lpValue. HPCON is typedef VOID*,
		// so the handle value IS a pointer. We use unsafe.Add to convert
		// the handle to unsafe.Pointer without triggering go vet's
		// unsafeptr analyzer (HPCON is a kernel handle, not a Go pointer).
		unsafe.Add(unsafe.Pointer(nil), uintptr(pconsole)),
		unsafe.Sizeof(pconsole),
	); err != nil {
		return nil, fmt.Errorf("pty: ProcThreadAttributeList.Update: %w", err)
	}

	// Build the command line string.
	cmdLine := buildCommandLine(cfg.Command, cfg.Args)
	cmdLinePtr, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		return nil, fmt.Errorf("pty: invalid command line: %w", err)
	}

	// Build the UTF-16 environment block.
	env := os.Environ()
	env = append(env, "TERM="+cfg.Term)
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	envBlock, err := createEnvBlock(env)
	if err != nil {
		return nil, fmt.Errorf("pty: invalid environment: %w", err)
	}

	// Working directory.
	var dirPtr *uint16
	if cfg.Dir != "" {
		dirPtr, err = syscall.UTF16PtrFromString(cfg.Dir)
		if err != nil {
			return nil, fmt.Errorf("pty: invalid directory: %w", err)
		}
	}

	// Create the child process with the extended startup info.
	//
	// STARTF_USESTDHANDLES (with hStd* left NULL) is REQUIRED here, and the
	// reason is subtle. With bInheritHandles=FALSE (below), Windows'
	// NtCreateUserProcess applies a legacy fallback that DUPLICATES the
	// parent's standard handles into a console-app child (a ConPTY child
	// qualifies). ConPTY's own cleanup (CONSOLE_USING_PTY_REFERENCE ->
	// ConsoleCloseIfConsoleHandle) only discards duplicated *console* handles,
	// NOT pipe/file handles — so when the parent's stdout is a pipe or file
	// (e.g. under `go test`, or any redirected/headless caller), the child
	// would inherit a working handle to the PARENT's stdout, bypass the
	// pseudoconsole entirely, and the ConPTY output pipe would never reach
	// EOF — deadlocking the read on child exit (reproduced: TestConPTY_Smoke
	// and TestCaptureAgentHandle_* fail/hang when this flag is removed).
	//
	// STARTF_USESTDHANDLES + NULL hStd* forces the child's standard-handle
	// slots to NULL, bypassing the duplication fallback; the pseudoconsole
	// attach then opens fresh ConDrv handles bound to conhost's screen buffer,
	// restoring correct output capture and a clean EOF on exit. This is the
	// canonical ConPTY pattern — Microsoft's own node-pty sets exactly this
	// combination ("VERY IMPORTANT that [bInheritHandles] is false"), as does
	// charmbracelet/x/conpty. The MS EchoCon sample omits the flag only
	// because it assumes a console-attached parent whose handles the cleanup
	// can discard. Refs: microsoft/terminal#15814 (DHowett, eryksun);
	// rprichard/win32-console-docs (CreateProcess rule 4 vs 6); vim/vim#19589.
	// Do NOT remove this flag without re-validating ConPTY capture on Windows.
	si := windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb:    uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			Flags: windows.STARTF_USESTDHANDLES,
		},
		ProcThreadAttributeList: attrList.List(),
	}
	var pi windows.ProcessInformation

	// SecurityAttributes with InheritHandle=1 for process/thread handles,
	// matching the charmbracelet/x/conpty reference implementation.
	pSec := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}
	tSec := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), InheritHandle: 1}

	if err := windows.CreateProcess(
		nil,        // lpApplicationName — parsed from cmdLine
		cmdLinePtr, // lpCommandLine
		pSec,       // lpProcessAttributes
		tSec,       // lpThreadAttributes
		false,      // bInheritHandles
		windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT,
		envBlock, // lpEnvironment
		dirPtr,   // lpCurrentDirectory
		&si.StartupInfo,
		&pi,
	); err != nil {
		return nil, fmt.Errorf("pty: CreateProcess %q: %w", cfg.Command, err)
	}

	// The thread handle is not needed.
	windows.CloseHandle(pi.Thread)

	// Wrap pipe handles as os.File for Go's I/O runtime.
	outputFile := os.NewFile(uintptr(outputRead), "|0")
	inputFile := os.NewFile(uintptr(inputWrite), "|1")

	handle := &windowsProcessHandle{
		process:   pi.Process,
		pid:       pi.ProcessId,
		conPTY:    pconsole,
		inputRead: inputRead,
	}
	done := make(chan struct{})

	proc := &Process{
		ptyFile:           outputFile,
		writeFile:         inputFile,
		done:              done,
		cmd:               handle,
		exitCode:          -1,
		writeTimeout:      cfg.WriteTimeout,
		closeGracefulWait: cfg.CloseGracePeriod,
		closeForceWait:    cfg.CloseForceWait,
	}

	// Background goroutine: wait for process exit and extract exit code.
	// NOTE: The process handle is NOT closed here — platformClose() handles
	// that after Close() has finished signaling, preventing use-after-close
	// races between Signal() calls and handle cleanup.
	go func() {
		defer close(done)
		waitErr := handle.Wait()

		var exitCode uint32
		if waitErr == nil {
			_ = windows.GetExitCodeProcess(handle.process, &exitCode)
		}

		proc.mu.Lock()
		defer proc.mu.Unlock()
		if waitErr != nil {
			proc.exitCode = -1
			proc.exitErr = waitErr
		} else {
			proc.exitCode = int(exitCode)
		}
	}()

	// Context cancellation goroutine.
	go func() {
		select {
		case <-ctx.Done():
			_ = handle.Signal(syscall.SIGKILL)
		case <-done:
		}
	}()

	return proc, nil
}

// platformResize resizes the ConPTY using ResizePseudoConsole.
// Must be called with p.mu held.
func (p *Process) platformResize(rows, cols uint16) error {
	h, ok := p.cmd.(*windowsProcessHandle)
	if !ok {
		return ErrNotSupported
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conPTY == 0 {
		return ErrNotSupported
	}
	size := windows.Coord{X: int16(cols), Y: int16(rows)}
	return windows.ResizePseudoConsole(h.conPTY, size)
}

// platformClose releases the ConPTY and process handles. Called by
// Close() AFTER <-p.done ensures the wait goroutine has finished,
// making the process handle safe to close.
func (p *Process) platformClose() {
	if h, ok := p.cmd.(*windowsProcessHandle); ok {
		h.closeConPTY()
		if h.process != 0 {
			_ = windows.CloseHandle(h.process)
			h.process = 0
		}
	}
}

// platformClosePseudoConsole closes the ConPTY pseudoconsole handle
// without closing the process or output file. This triggers the ConPTY
// to flush its internal buffer to the output pipe and close the pipe's
// write end, causing Read() to return remaining data followed by EOF.
// Thread-safe via closeConPTY's internal mutex.
func (p *Process) platformClosePseudoConsole() {
	if h, ok := p.cmd.(*windowsProcessHandle); ok {
		h.closeConPTY()
	}
}

// buildCommandLine constructs a Windows command line string.
// When args is empty, name is returned as-is so that callers can
// pass pre-formed command lines like "cmd.exe /c echo hello".
// When args is provided, each element is escaped per Windows rules.
func buildCommandLine(name string, args []string) string {
	if len(args) == 0 {
		return name
	}
	var b strings.Builder
	b.WriteString(syscall.EscapeArg(name))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(syscall.EscapeArg(a))
	}
	return b.String()
}

// createEnvBlock builds a UTF-16 encoded environment block for
// CreateProcess with CREATE_UNICODE_ENVIRONMENT. Each entry is
// null-terminated, and the block ends with a double null terminator.
func createEnvBlock(env []string) (*uint16, error) {
	if len(env) == 0 {
		block := []uint16{0, 0}
		return &block[0], nil
	}
	env = dedupEnv(env)
	slices.SortFunc(env, func(a, b string) int {
		aKey, _, _ := strings.Cut(a, "=")
		bKey, _, _ := strings.Cut(b, "=")
		return strings.Compare(strings.ToUpper(aKey), strings.ToUpper(bKey))
	})
	var b []uint16
	for _, s := range env {
		u, err := syscall.UTF16FromString(s)
		if err != nil {
			return nil, err
		}
		b = append(b, u...) // includes null terminator per entry
	}
	b = append(b, 0) // double null terminator ending the block
	return &b[0], nil
}

func dedupEnv(env []string) []string {
	out := make([]string, 0, len(env))
	seen := make(map[string]struct{}, len(env))
	for i := len(env) - 1; i >= 0; i-- {
		key, _, _ := strings.Cut(env[i], "=")
		folded := strings.ToUpper(key)
		if _, ok := seen[folded]; ok {
			continue
		}
		seen[folded] = struct{}{}
		out = append(out, env[i])
	}
	slices.Reverse(out)
	return out
}
