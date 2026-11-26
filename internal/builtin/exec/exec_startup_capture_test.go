package exec

import (
	"context"
	"errors"
	"io"
	"os"
	osexec "os/exec"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type rejectExitedTree struct {
	processTree
	cmd    *osexec.Cmd
	exited atomic.Bool
}

func (tree *rejectExitedTree) attach(cmd *osexec.Cmd) error {
	tree.cmd = cmd
	if err := tree.processTree.attach(cmd); err != nil {
		return err
	}
	observed, err := waitProcessBeforeReap(cmd)
	if err != nil {
		return err
	}
	if !observed {
		return errors.New("fixture could not observe the child before reap")
	}
	tree.exited.Store(true)
	return errors.New("post-start attach failure")
}

// unreadOutputObserver inspects only the bytes production cleanup is about
// to discard. The child writes to the original file; the observer does not
// read until Close, after the child has exited and both writers have closed.
type unreadOutputObserver struct {
	commandPipeEndpoint
	exited *atomic.Bool
	mu     sync.Mutex
	closed bool
	unread string
	err    error
}

func (p *unreadOutputObserver) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.closed {
		p.closed = true
		if p.exited.Load() {
			data, err := io.ReadAll(p.commandPipeEndpoint)
			p.unread, p.err = string(data), err
		}
	}
	return p.commandPipeEndpoint.Close()
}

func TestRunExec_PostStartFailureReportsIncompleteCapture(t *testing.T) {
	skipSlow(t)
	t.Setenv(outputLifecycleHelperEnv, "ascii")
	t.Setenv("GORACE", "atexit_sleep_ms=0")

	native, err := newProcessTree()
	if err != nil {
		t.Fatal(err)
	}
	tree := &rejectExitedTree{processTree: native}
	var stdout *unreadOutputObserver
	newPipe := func() (commandPipeEndpoint, commandPipeEndpoint, error) {
		reader, writer, err := newCommandPipe()
		if err != nil {
			return reader, writer, err
		}
		if stdout == nil {
			stdout = &unreadOutputObserver{commandPipeEndpoint: reader, exited: &tree.exited}
			return stdout, writer, nil
		}
		return reader, writer, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const payload = "0123456789abcdef"
	result := runExecCommand(ctx, SpawnConfig{Command: os.Args[0]}, newPipe, func() (processTree, error) { return tree, nil }, outputEOFCeiling)
	if tree.cmd == nil || tree.cmd.ProcessState == nil || tree.cmd.ProcessState.ExitCode() != 0 {
		t.Fatalf("fixture did not record a successful child exit: cmd=%v, result=%v", tree.cmd, result)
	}
	if result["code"] != 0 {
		t.Fatalf("post-start rollback changed the recorded exit status: result=%v", result)
	}
	delivered, ok := result["stdout"].(string)
	if !ok {
		t.Fatalf("stdout result has unexpected type: %T", result["stdout"])
	}
	stdout.mu.Lock()
	unread, inspectErr := stdout.unread, stdout.err
	stdout.mu.Unlock()
	if inspectErr != nil {
		t.Fatalf("inspecting unread output at Close: %v", inspectErr)
	}
	if delivered+unread != payload {
		t.Fatalf("emitted bytes not accounted for: delivered=%q unreadAtClose=%q want=%q", delivered, unread, payload)
	}
	if result["error"] != true || (delivered != payload && result["truncated"] != true) {
		t.Fatalf("post-start failure lost output without marking it incomplete: unreadAtClose=%q result=%v", unread, result)
	}
}
