package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// cleanupFaultTree injects teardown failures without leaking native owners.
// A real backend is closed once; retries observe only the injected outcome.
// A nil backend is used only with an already-reaped, single-process fixture.
type cleanupFaultTree struct {
	processTree
	closeErr       error
	transient      bool
	attachErr      error
	closeInAttach  bool
	closedInAttach bool
	beforeClose    func()
	nativeClose    sync.Once
	nativeCloseErr error
	closeCalls     atomic.Int32
	killCalls      atomic.Int32
	cmd            *osexec.Cmd
}

func (t *cleanupFaultTree) attach(cmd *osexec.Cmd) error {
	t.cmd = cmd
	if t.attachErr == nil {
		if t.processTree != nil {
			return t.processTree.attach(cmd)
		}
		return nil
	}
	if !t.closeInAttach {
		return t.attachErr
	}
	if t.processTree != nil {
		if err := t.processTree.attach(cmd); err != nil {
			return err
		}
	}
	closeErr := t.close(cmd)
	t.closedInAttach = true
	if closeErr != nil {
		return errors.Join(t.attachErr, &processTreeCloseError{err: closeErr})
	}
	return t.attachErr
}

func (t *cleanupFaultTree) kill(cmd *osexec.Cmd) error {
	t.killCalls.Add(1)
	if t.processTree != nil {
		return t.processTree.kill(cmd)
	}
	return nil
}

func (t *cleanupFaultTree) close(cmd *osexec.Cmd) error {
	t.cmd = cmd
	if t.closedInAttach {
		return nil
	}
	attempt := t.closeCalls.Add(1)
	t.nativeClose.Do(func() {
		if t.processTree != nil {
			t.nativeCloseErr = t.processTree.close(cmd)
		}
	})
	if t.beforeClose != nil {
		t.beforeClose()
	}
	injected := t.closeErr
	if t.transient && attempt > 1 {
		injected = nil
	}
	return errors.Join(t.nativeCloseErr, injected)
}

func cleanupTreeFactory(t *testing.T, tree *cleanupFaultTree) processTreeFactory {
	t.Helper()
	native, err := newProcessTree()
	if err != nil {
		t.Fatal(err)
	}
	tree.processTree = native
	return func() (processTree, error) { return tree, nil }
}

func requireCleanupMetadata(t *testing.T, values map[string]any, want bool) {
	t.Helper()
	value, exists := values["treeCleanupError"]
	if !want {
		if exists {
			t.Fatalf("unexpected treeCleanupError=%v in %v", value, values)
		}
		return
	}
	message, ok := value.(string)
	if !ok || !strings.Contains(message, syscall.EPERM.Error()) {
		t.Fatalf("treeCleanupError=%v, want the injected native failure in %v", value, values)
	}
}

func TestChildWait_TreeCleanupErrorPreservesStatus(t *testing.T) {
	skipSlow(t)
	for _, code := range []int{0, 7} {
		for _, mode := range []string{"clean", "failed", "retry_recovered", "pipe_error_only"} {
			t.Run(fmt.Sprintf("exit_%d/%s", code, mode), func(t *testing.T) {
				command, args := exitCodeCommand(code)
				cmd := osexec.Command(command, args...)
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				tree := &cleanupFaultTree{}
				if mode == "failed" || mode == "retry_recovered" {
					tree.closeErr = syscall.EPERM
					tree.transient = mode == "retry_recovered"
				}
				waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, func(*osexec.Cmd) (bool, error) {
					return false, nil
				})
				if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != code {
					t.Fatalf("real process state=%v, want exit %d", cmd.ProcessState, code)
				}
				if mode == "pipe_error_only" {
					waitErr = errors.Join(waitErr, errors.New("pipe close after EOF"))
				}
				child := &ChildProcess{
					cmd: cmd, tree: tree, done: make(chan struct{}), abort: make(chan struct{}),
					stdout: newPipeQueue(0), stderr: newPipeQueue(0),
				}
				child.pumpPipe(io.NopCloser(strings.NewReader("")), &child.stdout)
				child.pumpPipe(io.NopCloser(strings.NewReader("")), &child.stderr)
				child.recordExit(waitErr, closeErr)
				close(child.done)
				_, goErr := child.WaitContext(context.Background())
				if errors.Is(goErr, syscall.EPERM) != (mode == "failed") {
					t.Fatalf("Go wait error=%v, mode=%s", goErr, mode)
				}

				vm, adapter, loop := newJSTestRuntime(t)
				execTestOwner(t, loop, func() error {
					return vm.Set("child", wrapChildProcess(context.Background(), vm, adapter, loop, child, func() {}))
				})
				got := evalOnLoop(t, vm, loop, `(async function () {
					var first = await child.wait();
					var stdout = await child.stdout.read();
					var stderr = await child.stderr.read();
					await child.kill();
					var second = await child.wait();
					__got({wire: JSON.stringify({first: first, second: second, stdout: stdout, stderr: stderr})});
				})().catch(function (e) { __got({error: String(e)}); });`)
				wire, ok := got["wire"].(string)
				if !ok {
					t.Fatalf("known child status did not resolve through JS: %v", got)
				}
				var observed struct {
					First, Second  map[string]any
					Stdout, Stderr struct{ Done, Truncated bool }
				}
				if err := json.Unmarshal([]byte(wire), &observed); err != nil {
					t.Fatal(err)
				}
				for _, status := range []map[string]any{observed.First, observed.Second} {
					if status["code"] != float64(code) || status["signal"] != nil {
						t.Fatalf("known status changed: %s", wire)
					}
					requireCleanupMetadata(t, status, mode == "failed")
				}
				if !observed.Stdout.Done || observed.Stdout.Truncated || !observed.Stderr.Done || observed.Stderr.Truncated {
					t.Fatalf("tree failure changed independently recorded clean EOF: %s", wire)
				}
				if tree.killCalls.Load() != 0 {
					t.Fatalf("reaped child was signalled again %d times", tree.killCalls.Load())
				}
			})
		}
	}
}

type cleanupEOFReader struct {
	closeErr error
	closed   chan struct{}
	once     sync.Once
}

func (*cleanupEOFReader) Read([]byte) (int, error)    { return 0, io.EOF }
func (*cleanupEOFReader) Write(p []byte) (int, error) { return len(p), nil }
func (r *cleanupEOFReader) Close() error {
	r.once.Do(func() { close(r.closed) })
	return r.closeErr
}

func TestRunExec_TreeCleanupErrorKeepsCodeAndCapture(t *testing.T) {
	skipSlow(t)
	for _, code := range []int{0, 7} {
		for _, mode := range []string{"clean", "failed", "retry_recovered", "pipe_error_only", "tree_and_pipe_error"} {
			t.Run(fmt.Sprintf("exit_%d/%s", code, mode), func(t *testing.T) {
				stdout := &cleanupEOFReader{closed: make(chan struct{})}
				stderr := &cleanupEOFReader{closed: make(chan struct{})}
				pipeError := mode == "pipe_error_only" || mode == "tree_and_pipe_error"
				if pipeError {
					stdout.closeErr = errors.New("output pipe close after EOF")
				}
				pipes := 0
				newPipe := func() (commandPipeEndpoint, commandPipeEndpoint, error) {
					pipes++
					if pipes == 1 {
						return stdout, &discardWriter{}, nil
					}
					return stderr, &discardWriter{}, nil
				}
				tree := &cleanupFaultTree{beforeClose: func() { <-stdout.closed; <-stderr.closed }}
				wantTreeError := mode == "failed" || mode == "tree_and_pipe_error"
				if wantTreeError || mode == "retry_recovered" {
					tree.closeErr = syscall.EPERM
					tree.transient = mode == "retry_recovered"
				}
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				command, args := exitCodeCommand(code)
				result := runExecCommand(ctx, command, args, newPipe, cleanupTreeFactory(t, tree), outputEOFCeiling)
				if result["code"] != code || result["truncated"] != false {
					t.Fatalf("observed code/EOF changed: %v", result)
				}
				if result["error"] != (code != 0 || wantTreeError || pipeError) {
					t.Fatalf("error classification changed: %v", result)
				}
				requireCleanupMetadata(t, result, wantTreeError)
				if pipeError && !strings.Contains(result["message"].(string), stdout.closeErr.Error()) {
					t.Fatalf("pipe error disappeared: %v", result)
				}
			})
		}
	}
}

func TestRunExec_TreeCleanupErrorOnStartupFailure(t *testing.T) {
	skipSlow(t)
	for _, mode := range []string{"writer_close", "attach_close", "attach_already_closed", "attach_cleanup_succeeded"} {
		t.Run(mode, func(t *testing.T) {
			tree := &cleanupFaultTree{}
			wantTreeError := mode != "attach_cleanup_succeeded"
			if wantTreeError {
				tree.closeErr = syscall.EPERM
			}
			if mode != "writer_close" {
				tree.attachErr = errors.New("attach failed")
				tree.closeInAttach = mode == "attach_already_closed"
			}
			newPipe := func() (commandPipeEndpoint, commandPipeEndpoint, error) {
				writer := &failingCommandPipe{}
				if mode == "writer_close" {
					writer.closeErr = errors.New("writer close failed")
				}
				return &failingCommandPipe{}, writer, nil
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command, args := immediateExitCommand()
			result := runExecCommand(ctx, command, args, newPipe, cleanupTreeFactory(t, tree), outputEOFCeiling)
			if result["error"] != true || tree.cmd == nil || tree.cmd.ProcessState == nil {
				t.Fatalf("fixture did not reap the started child: result=%v, cmd=%v", result, tree.cmd)
			}
			requireCleanupMetadata(t, result, wantTreeError)
			if mode == "attach_already_closed" && tree.closeCalls.Load() != 1 {
				t.Fatalf("attach failure was not the only real close: %d", tree.closeCalls.Load())
			}
		})
	}
}

func TestSpawn_TreeCleanupErrorOnRejectedStartup(t *testing.T) {
	skipSlow(t)
	for _, mode := range []string{"attach_close", "attach_already_closed", "attach_cleanup_succeeded", "not_started"} {
		t.Run(mode, func(t *testing.T) {
			tree := &cleanupFaultTree{attachErr: errors.New("attach failed")}
			wantTreeError := mode == "attach_close" || mode == "attach_already_closed"
			if mode != "attach_cleanup_succeeded" {
				tree.closeErr = syscall.EPERM
			}
			tree.closeInAttach = mode == "attach_already_closed"
			factory := cleanupTreeFactory(t, tree)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			command, args := immediateExitCommand()
			if mode == "not_started" {
				command, args = filepath.Join(t.TempDir(), "missing-executable"), nil
			}
			vm, adapter, loop := newJSTestRuntime(t)
			execTestOwner(t, loop, func() error {
				spawn := func(ctx context.Context, cfg SpawnConfig) (*ChildProcess, error) {
					return spawnChild(ctx, cfg, factory)
				}
				if err := vm.Set("spawn", jsSpawn(ctx, vm, adapter, loop, spawn)); err != nil {
					return err
				}
				if err := vm.Set("command", command); err != nil {
					return err
				}
				return vm.Set("args", args)
			})
			got := evalOnLoop(t, vm, loop, `(async function () {
				try {
					await spawn(command, args);
					__got({unexpectedSuccess: true});
				} catch (e) {
					var result = {message: e.message};
					if (Object.prototype.hasOwnProperty.call(e, "treeCleanupError")) result.treeCleanupError = e.treeCleanupError;
					__got(result);
				}
			})();`)
			if got["unexpectedSuccess"] == true || !strings.Contains(fmt.Sprint(got["message"]), "spawn failed") {
				t.Fatalf("spawn did not reject with its original error: %v", got)
			}
			requireCleanupMetadata(t, got, wantTreeError)
			if mode != "not_started" && (tree.cmd == nil || tree.cmd.ProcessState == nil) {
				t.Fatalf("post-start rejection did not reap the real child: %v", tree.cmd)
			}
		})
	}
}

func TestRunExec_PreStartCleanupErrorHasNoTreeMetadata(t *testing.T) {
	skipSlow(t)
	tree := &cleanupFaultTree{closeErr: syscall.EPERM}
	command := filepath.Join(t.TempDir(), "missing-executable")
	result := runExecCommand(context.Background(), command, nil, newCommandPipe, cleanupTreeFactory(t, tree), outputEOFCeiling)
	if result["error"] != true || result["code"] != -1 {
		t.Fatalf("pre-start failure=%v", result)
	}
	requireCleanupMetadata(t, result, false)
}

func TestChildWait_RejectionRetainsTreeCleanupError(t *testing.T) {
	skipSlow(t)
	child := &ChildProcess{cmd: &osexec.Cmd{}, done: make(chan struct{}), abort: make(chan struct{})}
	child.recordExit(errors.New("no observed child status"), syscall.EPERM)
	close(child.done)
	vm, adapter, loop := newJSTestRuntime(t)
	execTestOwner(t, loop, func() error {
		return vm.Set("child", wrapChildProcess(context.Background(), vm, adapter, loop, child, func() {}))
	})
	got := evalOnLoop(t, vm, loop, `child.wait().then(function () { __got({unexpectedSuccess: true}); }, function (e) {
		__got({message: e.message, treeCleanupError: e.treeCleanupError});
	});`)
	if got["unexpectedSuccess"] == true || !strings.Contains(fmt.Sprint(got["message"]), "no observed child status") {
		t.Fatalf("wait rejection changed: %v", got)
	}
	requireCleanupMetadata(t, got, true)
}
