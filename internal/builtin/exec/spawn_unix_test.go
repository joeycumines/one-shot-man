//go:build unix

package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

func TestKillProcessGroup_ReportsPermissionFailure(t *testing.T) {
	t.Parallel()
	cmd := &osexec.Cmd{Process: &os.Process{Pid: 42}}
	err := killProcessGroup(cmd, func(pid int, signal syscall.Signal) error {
		if pid != -42 {
			t.Errorf("kill pid = %d, want -42", pid)
		}
		if signal != syscall.SIGKILL {
			t.Errorf("kill signal = %v, want SIGKILL", signal)
		}
		return syscall.EPERM
	})
	if !errors.Is(err, syscall.EPERM) {
		t.Fatalf("killProcessGroup error = %v, want EPERM", err)
	}
}

func TestProcessTreeClose_AfterReapOnlyObserves(t *testing.T) {
	for _, tc := range []struct {
		name         string
		observations []error
		wantErr      bool
		wantCause    error
		wantElapsed  time.Duration
	}{
		{name: "absent", observations: []error{syscall.ESRCH}},
		{name: "zombies_reaped", observations: []error{syscall.EPERM, syscall.EPERM, syscall.ESRCH}, wantElapsed: 20 * time.Millisecond},
		{name: "live_group_exits", observations: []error{nil, syscall.ESRCH}, wantElapsed: 10 * time.Millisecond},
		{name: "permission_denied", observations: []error{syscall.EPERM}, wantErr: true, wantCause: syscall.EPERM, wantElapsed: time.Second},
		{name: "live_group_remains", observations: []error{nil}, wantErr: true, wantElapsed: time.Second},
		{name: "unknown_probe_failure", observations: []error{syscall.EIO}, wantErr: true, wantCause: syscall.EIO},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				tree := &unixProcessTree{}
				tree.beginReap(false)
				cmd := &osexec.Cmd{Process: &os.Process{Pid: 42}}
				calls := 0
				probe := func(pid int, signal syscall.Signal) error {
					if pid != -42 || signal != 0 {
						t.Fatalf("post-reap probe attempted pid=%d signal=%v; former groups must never be signalled", pid, signal)
					}
					index := min(calls, len(tc.observations)-1)
					calls++
					return tc.observations[index]
				}
				start := time.Now()
				err := tree.closeGroup(cmd, probe)
				if (err != nil) != tc.wantErr {
					t.Fatalf("close = %v, want error=%v", err, tc.wantErr)
				}
				if tc.wantCause != nil && !errors.Is(err, tc.wantCause) {
					t.Fatalf("close = %v, want original cause %v", err, tc.wantCause)
				}
				if elapsed := time.Since(start); elapsed != tc.wantElapsed {
					t.Fatalf("observation took %v, want %v", elapsed, tc.wantElapsed)
				}
				if repeated := tree.closeGroup(cmd, func(int, syscall.Signal) error {
					t.Fatal("repeated close restarted observation")
					return nil
				}); repeated != err {
					t.Fatalf("repeated close changed its first result: %v -> %v", err, repeated)
				}
			})
		})
	}
}

// Both group members are direct children of this test, so their reap times
// remain under its control. An unreaped nonleader keeps the group present
// after its leader is reaped; no timing assumption about init is involved.
func TestProcessTreeClose_DarwinGroupReap(t *testing.T) {
	skipSlow(t)
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin native zombie process-group behavior")
	}
	for _, mode := range []string{"all_reaped", "zombie_remains", "live_member_remains"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			start := func(pgid int) (*osexec.Cmd, func()) {
				cmd := osexec.CommandContext(ctx, "sh", "-c", "read input; exit 0")
				cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: pgid}
				stdin, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					closeErr := stdin.Close()
					t.Fatal(errors.Join(err, closeErr))
				}
				t.Cleanup(func() {
					if err := stdin.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
						t.Errorf("close helper stdin: %v", err)
					}
					if cmd.ProcessState == nil {
						if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
							t.Errorf("kill helper: %v", err)
						}
						var exitErr *osexec.ExitError
						if err := cmd.Wait(); err != nil && !errors.As(err, &exitErr) {
							t.Errorf("reap helper: %v", err)
						}
					}
				})
				return cmd, func() {
					if err := stdin.Close(); err != nil {
						t.Fatal(err)
					}
					if observed, err := waitProcessBeforeReap(cmd); err != nil || !observed {
						t.Fatalf("observe exited helper before reap: observed=%v err=%v", observed, err)
					}
				}
			}
			leader, exitLeader := start(0)
			member, exitMember := start(leader.Process.Pid)
			exitLeader()
			tree, err := newProcessTree()
			if err != nil {
				t.Fatal(err)
			}
			if mode != "live_member_remains" {
				exitMember()
				// No process has been reaped yet. The real kernel signal
				// must not turn EPERM into success merely because both
				// observed children have exited.
				if err := tree.close(leader); !errors.Is(err, syscall.EPERM) {
					t.Fatalf("close unreaped group = %v, want Darwin EPERM", err)
				}
			}
			tree.beginReap(true)
			if err := leader.Wait(); err != nil {
				t.Fatalf("reap leader: %v", err)
			}
			if mode == "all_reaped" {
				if err := member.Wait(); err != nil {
					t.Fatalf("reap member: %v", err)
				}
			}
			closeErr := tree.close(leader)
			if mode == "all_reaped" {
				if closeErr != nil {
					t.Fatalf("close absent group: %v", closeErr)
				}
				return
			}
			if closeErr == nil {
				t.Fatal("existing group was incorrectly confirmed gone")
			}
			if mode == "zombie_remains" && !errors.Is(closeErr, syscall.EPERM) {
				t.Fatalf("zombie-group permission result was lost: %v", closeErr)
			}
			if mode == "live_member_remains" {
				// A SIGKILL after leader reap would make this child's Wait
				// report a signal instead of its deliberate successful exit.
				exitMember()
			}
			if err := member.Wait(); err != nil {
				t.Fatalf("remaining member was signalled after leader reap: %v", err)
			}
		})
	}
}

// gatedSignalTree replaces only the syscall boundary. The real Unix owner
// still owns the signal mutex and the transition that precedes Cmd.Wait.
type gatedSignalTree struct {
	*unixProcessTree
	deliver        func(int, syscall.Signal) error
	closeCall      func(int, syscall.Signal) error
	closeEntered   chan struct{}
	reapEntered    chan struct{}
	reapSealed     chan struct{}
	waitRelease    <-chan struct{}
	signalFinished *atomic.Bool
	t              *testing.T
}

func (tree *gatedSignalTree) kill(cmd *osexec.Cmd) error {
	return tree.signalGroup(cmd, syscall.SIGKILL, tree.deliver)
}
func (tree *gatedSignalTree) signal(cmd *osexec.Cmd, sig os.Signal) error {
	return tree.signalGroup(cmd, sig.(syscall.Signal), tree.deliver)
}
func (tree *gatedSignalTree) close(cmd *osexec.Cmd) error {
	if tree.closeEntered != nil {
		select {
		case tree.closeEntered <- struct{}{}:
		default:
		}
	}
	return tree.closeGroup(cmd, tree.closeCall)
}
func (tree *gatedSignalTree) beginReap(observed bool) {
	if tree.reapEntered != nil {
		close(tree.reapEntered)
	}
	tree.unixProcessTree.beginReap(observed)
	if tree.signalFinished != nil && !tree.signalFinished.Load() {
		tree.t.Error("reap gate sealed before an admitted group signal returned")
	}
	if tree.reapSealed != nil {
		close(tree.reapSealed)
	}
	if tree.waitRelease != nil {
		<-tree.waitRelease
	}
}

type gatedWaitResult struct{ waitErr, closeErr error }

func TestUnixProcessTree_AdmittedSignalPrecedesReap(t *testing.T) {
	skipSlow(t)
	for _, mode := range []string{"observed", "unsupported", "probe_error"} {
		for _, delivery := range []string{"child_signal", "child_kill", "command_cancel"} {
			t.Run(mode+"/"+delivery, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				entered, release := make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				unblock := func() { releaseOnce.Do(func() { close(release) }) }
				defer unblock()
				var signalFinished atomic.Bool
				tree := &gatedSignalTree{
					unixProcessTree: &unixProcessTree{}, t: t,
					closeEntered: make(chan struct{}, 1), reapEntered: make(chan struct{}), reapSealed: make(chan struct{}),
					signalFinished: &signalFinished,
				}
				cmd := newCommand(ctx, SpawnConfig{Command: "sh", Args: []string{"-c", "read input; exit 0"}})
				cmd.Cancel = func() error { return tree.kill(cmd) }
				stdin, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					_ = stdin.Close()
					t.Fatal(err)
				}
				var waiting bool
				reaped := make(chan gatedWaitResult, 1)
				reapDone := make(chan struct{})
				defer func() {
					unblock()
					_ = stdin.Close()
					if !waiting {
						_ = cmd.Process.Kill()
						_ = cmd.Wait()
					} else {
						select {
						case <-reapDone:
						case <-time.After(5 * time.Second):
							t.Error("cleanup could not join the reaper")
						}
					}
				}()
				var deliveryCalls atomic.Int32
				tree.deliver = func(pid int, sig syscall.Signal) error {
					if deliveryCalls.Add(1) != 1 {
						t.Error("unexpected additional group signal")
						return nil
					}
					if pid != -cmd.Process.Pid || (sig != syscall.SIGTERM && sig != syscall.SIGKILL) {
						t.Errorf("unexpected owned group signal pid=%d signal=%v", pid, sig)
					}
					if tree.mu.TryLock() {
						tree.mu.Unlock()
						t.Error("group syscall ran without holding the reap gate")
					}
					close(entered)
					<-release
					signalFinished.Store(true)
					return nil
				}
				tree.closeCall = func(pid int, sig syscall.Signal) error {
					if pid != -cmd.Process.Pid {
						t.Errorf("close addressed another group: %d", pid)
					}
					if tree.mu.TryLock() {
						tree.mu.Unlock()
						t.Error("close syscall ran without holding the reap gate")
					}
					if tree.reaping {
						if sig != 0 {
							t.Errorf("post-seal close delivered signal %v", sig)
						}
						return syscall.ESRCH
					}
					if sig != syscall.SIGKILL {
						t.Errorf("pre-reap close signal = %v", sig)
					}
					return nil
				}
				child := &ChildProcess{cmd: cmd, tree: tree, done: make(chan struct{})}
				delivered := make(chan error, 1)
				go func() {
					switch delivery {
					case "child_signal":
						delivered <- child.Signal("SIGTERM")
					case "child_kill":
						delivered <- child.Kill()
					case "command_cancel":
						delivered <- cmd.Cancel()
					}
				}()
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("signal did not reach its syscall barrier")
				}
				if mode == "observed" {
					if err := stdin.Close(); err != nil {
						t.Fatal(err)
					}
				}
				waiting = true
				go func() {
					defer close(reapDone)
					waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, func(cmd *osexec.Cmd) (bool, error) {
						switch mode {
						case "observed":
							return waitProcessBeforeReap(cmd)
						case "probe_error":
							return false, errors.New("probe unavailable")
						default:
							return false, nil
						}
					})
					reaped <- gatedWaitResult{waitErr, closeErr}
				}()
				// Every branch must close under the signal gate before either
				// natural reap or conservative fallback termination.
				ready := tree.closeEntered
				select {
				case <-ready:
				case <-ctx.Done():
					t.Fatal("reaper did not reach the gate")
				}
				unblock()
				select {
				case err := <-delivered:
					if err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal("signal did not finish")
				}
				select {
				case <-tree.reapSealed:
				case <-ctx.Done():
					t.Fatal("reap gate did not seal")
				}
				if mode != "observed" {
					if err := stdin.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
						t.Fatal(err)
					}
				}
				select {
				case result := <-reaped:
					if mode == "observed" {
						if result.waitErr != nil || result.closeErr != nil {
							t.Fatalf("wait=%v close=%v", result.waitErr, result.closeErr)
						}
					} else {
						var exitErr *osexec.ExitError
						if result.waitErr != nil && !errors.As(result.waitErr, &exitErr) {
							t.Fatal(result.waitErr)
						}
						if result.closeErr == nil {
							t.Fatal("fallback observation failure was lost")
						}
					}
				case <-ctx.Done():
					t.Fatal("Cmd.Wait did not finish after gate release")
				}
			})
		}
	}
}

func TestUnixProcessTree_FallbackRefusesSignalsAndJoinsCancellation(t *testing.T) {
	skipSlow(t)
	for _, probeFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("probe_error_%t", probeFails), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			tree := &gatedSignalTree{unixProcessTree: &unixProcessTree{}, reapSealed: make(chan struct{}), waitRelease: release}
			cmd := newCommand(ctx, SpawnConfig{Command: "sh", Args: []string{"-c", "read input; exit 0"}})
			tree.deliver = func(int, syscall.Signal) error {
				t.Error("sealed fallback delivered a numeric group signal")
				return nil
			}
			tree.closeCall = func(pid int, sig syscall.Signal) error {
				if pid != -cmd.Process.Pid || sig != syscall.SIGKILL || tree.reaping {
					t.Errorf("fallback teardown pid=%d signal=%v reaping=%v", pid, sig, tree.reaping)
				}
				return nil
			}
			cancelled := make(chan error, 1)
			cmd.Cancel = func() error { err := tree.kill(cmd); cancelled <- err; return err }
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				_ = stdin.Close()
				t.Fatal(err)
			}
			done := make(chan struct{})
			result := make(chan gatedWaitResult, 1)
			defer func() {
				unblock()
				_ = stdin.Close()
				cancel()
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("cleanup could not join fallback waiter")
				}
			}()
			go func() {
				defer close(done)
				waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, func(*osexec.Cmd) (bool, error) {
					if probeFails {
						return false, errors.New("probe unavailable")
					}
					return false, nil
				})
				result <- gatedWaitResult{waitErr, closeErr}
			}()
			select {
			case <-tree.reapSealed:
			case <-time.After(5 * time.Second):
				t.Fatal("fallback gate did not seal")
			}
			// Wait is parked at the gate, so this assertion has no concurrent
			// ProcessState writer. Never probe or kill the positive PID here.
			if cmd.ProcessState != nil {
				t.Fatal("fixture entered Cmd.Wait before testing the seal")
			}
			child := &ChildProcess{cmd: cmd, tree: tree, done: make(chan struct{})}
			if err := child.Signal("SIGTERM"); !errors.Is(err, errProcessGroupReaping) {
				t.Fatalf("sealed Signal=%v", err)
			}
			if err := child.Kill(); !errors.Is(err, errProcessGroupReaping) {
				t.Fatalf("sealed Kill=%v", err)
			}
			cancel()
			select {
			case err := <-cancelled:
				if !errors.Is(err, errProcessGroupReaping) {
					t.Fatalf("sealed Cancel=%v", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("cancellation blocked at the sealed gate")
			}
			unblock()
			select {
			case got := <-result:
				var exitErr *osexec.ExitError
				if !errors.As(got.waitErr, &exitErr) || processExitSignal(got.waitErr) != "SIGKILL" || got.closeErr == nil {
					t.Fatalf("fallback wait=%v close=%v", got.waitErr, got.closeErr)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("Cmd.Wait could not join completed cancellation")
			}
		})
	}
}

func TestUnixProcessTree_ObservedSealDoesNotSignalUnreapedChild(t *testing.T) {
	skipSlow(t)
	cmd := osexec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	if observed, err := waitProcessBeforeReap(cmd); err != nil || !observed {
		t.Fatalf("observe exit: observed=%v err=%v", observed, err)
	}
	tree := &unixProcessTree{}
	tree.beginReap(true)
	if cmd.ProcessState != nil {
		t.Fatal("fixture unexpectedly reaped the child before testing the gate")
	}
	for _, sig := range []syscall.Signal{syscall.SIGKILL, syscall.SIGTERM} {
		if err := tree.signalGroup(cmd, sig, func(int, syscall.Signal) error {
			t.Error("sealed owner sent a group signal before status publication")
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnixProcessTree_FallbackTerminatesOwnedGroupBeforeReap(t *testing.T) {
	skipSlow(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	start := func(pgid int) (*osexec.Cmd, io.WriteCloser) {
		cmd := osexec.CommandContext(ctx, "sh", "-c", "read input; exit 0")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pgid: pgid}
		in, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			_ = in.Close()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := in.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				t.Errorf("close owned helper stdin: %v", err)
			}
			if cmd.ProcessState == nil {
				if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					t.Errorf("kill owned helper: %v", err)
				}
				var exitErr *osexec.ExitError
				if err := cmd.Wait(); err != nil && !errors.As(err, &exitErr) {
					t.Errorf("reap owned helper: %v", err)
				}
			}
		})
		return cmd, in
	}
	leader, leaderIn := start(0)
	member, memberIn := start(leader.Process.Pid)
	tree := &unixProcessTree{}
	if err := leaderIn.Close(); err != nil {
		t.Fatal(err)
	}
	waitErr, closeErr := waitAndCloseProcessTreeWithProbe(leader, tree, func(*osexec.Cmd) (bool, error) { return false, errors.New("probe unavailable") })
	var leaderExit *osexec.ExitError
	if waitErr != nil && !errors.As(waitErr, &leaderExit) {
		t.Fatal(waitErr)
	}
	if closeErr == nil {
		t.Fatal("failed observation was reported as ordinary completion")
	}
	if err := memberIn.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		t.Fatal(err)
	}
	err := member.Wait()
	var memberExit *osexec.ExitError
	if !errors.As(err, &memberExit) || processExitSignal(err) != "SIGKILL" {
		t.Fatalf("owned group member was not terminated before reap: %v", err)
	}
}

const runCommandEscapedWriterEnv = "OSM_TEST_RUN_COMMAND_ESCAPED_WRITER"

func TestRunCommandEscapedWriterHelper(t *testing.T) {
	if os.Getenv(runCommandEscapedWriterEnv) != "1" {
		return
	}
	if _, err := syscall.Setsid(); err != nil {
		os.Exit(91)
	}
	if err := os.WriteFile(os.Getenv("OSM_TEST_RUN_COMMAND_READY"), []byte("ready"), 0o600); err != nil {
		os.Exit(92)
	}
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		os.Exit(93)
	}
	if err := os.WriteFile(os.Getenv("OSM_TEST_RUN_COMMAND_DONE"), []byte("done"), 0o600); err != nil {
		os.Exit(94)
	}
	os.Exit(0)
}

func TestRunCommandWaitDelayBoundsEscapedWriter(t *testing.T) {
	skipSlow(t)
	dir := t.TempDir()
	readyPath, donePath := filepath.Join(dir, "ready"), filepath.Join(dir, "done")
	t.Setenv(runCommandEscapedWriterEnv, "1")
	t.Setenv("OSM_TEST_RUN_COMMAND_READY", readyPath)
	t.Setenv("OSM_TEST_RUN_COMMAND_DONE", donePath)
	t.Setenv(outputLifecycleHelperEnv, "")
	t.Setenv(escaperHelperEnv, "")
	t.Setenv(highVolumeWriterEnv, "")
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	input, release, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	// This independent fixture bound also releases the writer if the tested
	// WaitDelay is accidentally removed. It never signals a numeric PID.
	forceRelease := time.AfterFunc(10*time.Second, func() { _ = release.Close() })
	defer forceRelease.Stop()
	t.Cleanup(func() {
		_ = release.Close()
		_ = input.Close()
		if _, err := os.Stat(readyPath); errors.Is(err, os.ErrNotExist) {
			return
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(donePath); err == nil {
				return
			} else if !errors.Is(err, os.ErrNotExist) {
				t.Error(err)
				return
			}
			if time.Now().After(deadline) {
				t.Error("escaped helper did not finish after closing its owned input")
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
	})
	script := fmt.Sprintf("%s -test.run=^TestRunCommandEscapedWriterHelper$ <&3 & i=0; while [ ! -s %s ] && [ $i -lt 500 ]; do sleep 0.01; i=$((i+1)); done; test -s %s || exit 95; printf 'parent-done\\n'", shellQuote(os.Args[0]), shellQuote(readyPath), shellQuote(readyPath))
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := osexec.CommandContext(ctx, "sh", "-c", script)
	cmd.ExtraFiles = []*os.File{input}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	cmd.WaitDelay = 20 * time.Millisecond
	err = RunCommand(ctx, cmd)
	if !errors.Is(err, osexec.ErrWaitDelay) {
		t.Fatalf("RunCommand=%v, want bounded incomplete output drain; output=%q", err, output.String())
	}
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0 {
		t.Fatalf("parent did not exit successfully: %v", cmd.ProcessState)
	}
	if cmd.WaitDelay != 20*time.Millisecond {
		t.Fatal("WaitDelay was not restored")
	}
	assertRunCommandNoWatchdog(t, cmd)
}
