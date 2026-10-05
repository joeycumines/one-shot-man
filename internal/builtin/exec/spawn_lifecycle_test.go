package exec

import (
	"context"
	"errors"
	"io"
	"os"
	osexec "os/exec"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

type trackingReadCloser struct {
	closed chan struct{}
	once   sync.Once
}

func (r *trackingReadCloser) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (r *trackingReadCloser) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

type failingCommandPipe struct {
	closeErr   error
	closeCalls int
}

func (p *failingCommandPipe) Read([]byte) (int, error) {
	return 0, io.EOF
}

func (p *failingCommandPipe) Write(data []byte) (int, error) {
	return len(data), nil
}

func (p *failingCommandPipe) Close() error {
	p.closeCalls++
	return p.closeErr
}

type failingKillProcessTree struct {
	killStarted chan struct{}
}

func (*failingKillProcessTree) attach(*osexec.Cmd) error { return nil }
func (t *failingKillProcessTree) kill(*osexec.Cmd) error {
	close(t.killStarted)
	return os.ErrPermission
}
func (*failingKillProcessTree) close(*osexec.Cmd) error { return nil }

func TestCloseCommandPipeClearsPointerOnError(t *testing.T) {
	t.Parallel()

	closeErr := errors.New("close failed")
	pipe := &failingCommandPipe{closeErr: closeErr}
	var endpoint commandPipeEndpoint = pipe
	if err := closeCommandPipe(&endpoint); !errors.Is(err, closeErr) {
		t.Fatalf("first close error = %v, want %v", err, closeErr)
	}
	if endpoint != nil {
		t.Fatal("closeCommandPipe retained a closed endpoint")
	}
	if err := closeCommandPipe(&endpoint); err != nil {
		t.Fatalf("second close error = %v, want nil", err)
	}
	if pipe.closeCalls != 1 {
		t.Fatalf("close calls = %d, want 1", pipe.closeCalls)
	}
}

func TestCloseCommandPipesClosesAllOnError(t *testing.T) {
	t.Parallel()

	stdoutErr := errors.New("stdout close")
	stderrErr := errors.New("stderr close")
	stdout := &failingCommandPipe{closeErr: stdoutErr}
	stderr := &failingCommandPipe{closeErr: stderrErr}
	var stdoutReader, stdoutWriter commandPipeEndpoint = stdout, &failingCommandPipe{}
	var stderrReader, stderrWriter commandPipeEndpoint = stderr, &failingCommandPipe{}

	got := closeCommandPipes(&stdoutReader, &stdoutWriter, &stderrReader, &stderrWriter)
	if !errors.Is(got, stdoutErr) || !errors.Is(got, stderrErr) {
		t.Fatalf("closeCommandPipes error = %v, want both endpoint errors", got)
	}
	if stdoutReader != nil || stdoutWriter != nil || stderrReader != nil || stderrWriter != nil {
		t.Fatal("closeCommandPipes retained a closed endpoint")
	}
	if stdout.closeCalls != 1 || stderr.closeCalls != 1 {
		t.Fatalf("close calls = (%d, %d), want (1, 1)", stdout.closeCalls, stderr.closeCalls)
	}
	if err := closeCommandPipes(&stdoutReader, &stdoutWriter, &stderrReader, &stderrWriter); err != nil {
		t.Fatalf("second closeCommandPipes error = %v, want nil", err)
	}
}

func TestRunExecWriterCloseFailurePreservesCleanupErrors(t *testing.T) {
	skipSlow(t)

	stdoutErr := errors.New("stdout writer close")
	stderrErr := errors.New("stderr writer close")
	var endpoints []*failingCommandPipe
	newPipe := func() (commandPipeEndpoint, commandPipeEndpoint, error) {
		reader := &failingCommandPipe{}
		var writerErr error
		if len(endpoints) == 0 {
			writerErr = stdoutErr
		} else {
			writerErr = stderrErr
		}
		writer := &failingCommandPipe{closeErr: writerErr}
		endpoints = append(endpoints, reader, writer)
		return reader, writer, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command, args := hangingCommand()
	result := runExecWithPipeFactory(ctx, command, args, newPipe)
	if result["error"] != true {
		t.Fatalf("runExec result = %#v, want error", result)
	}
	message, _ := result["message"].(string)
	for _, target := range []string{stdoutErr.Error(), stderrErr.Error()} {
		if !strings.Contains(message, target) {
			t.Fatalf("runExec message = %q, want %q", message, target)
		}
	}
	if strings.Contains(message, "file already closed") {
		t.Fatalf("runExec message contains double-close noise: %q", message)
	}
	if len(endpoints) != 4 {
		t.Fatalf("pipe endpoints = %d, want 4", len(endpoints))
	}
	for i, endpoint := range endpoints {
		if endpoint.closeCalls != 1 {
			t.Fatalf("endpoint %d close calls = %d, want 1", i, endpoint.closeCalls)
		}
	}
}

func TestCleanupFailedRunExecOrdersAndPreservesErrors(t *testing.T) {
	t.Parallel()

	writerErr := errors.New("writer close")
	killErr := errors.New("kill")
	closeTreeErr := errors.New("close tree")
	waitErr := errors.New("wait")
	closePipesErr := errors.New("close pipes")
	events := make([]string, 0, 4)

	got := cleanupFailedRunExec(
		writerErr,
		func() error {
			events = append(events, "kill")
			return errors.Join(killErr, os.ErrProcessDone)
		},
		func() error {
			events = append(events, "close-tree")
			return closeTreeErr
		},
		func() error {
			events = append(events, "wait")
			return waitErr
		},
		func() error {
			events = append(events, "close-pipes")
			return closePipesErr
		},
	)

	wantEvents := []string{"kill", "close-tree", "wait", "close-pipes"}
	if len(events) != len(wantEvents) {
		t.Fatalf("cleanup events = %v, want %v", events, wantEvents)
	}
	for i := range wantEvents {
		if events[i] != wantEvents[i] {
			t.Fatalf("cleanup events = %v, want %v", events, wantEvents)
		}
	}
	for _, target := range []error{writerErr, killErr, closeTreeErr, waitErr, closePipesErr} {
		if !errors.Is(got, target) {
			t.Fatalf("cleanup error %v does not contain %v", got, target)
		}
	}
	if errors.Is(got, os.ErrProcessDone) {
		t.Fatal("cleanup error retained benign process-done error")
	}
}

func TestChildProcessPumpClosesPipe(t *testing.T) {
	t.Parallel()

	pipe := &trackingReadCloser{closed: make(chan struct{})}
	queue := newPipeQueue()
	child := &ChildProcess{}
	done := make(chan struct{})
	go func() {
		child.pumpPipe(pipe, &queue)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pumpPipe did not finish")
	}
	select {
	case <-pipe.closed:
	default:
		t.Fatal("pumpPipe did not close its reader")
	}
}

func TestSpawnChild_KillAfterExitIsNoOp(t *testing.T) {
	t.Parallel()

	command := "sh"
	args := []string{"-c", "exit 0"}
	if runtime.GOOS == "windows" {
		command = "cmd.exe"
		args = []string{"/C", "exit 0"}
	}

	child, err := SpawnChild(context.Background(), SpawnConfig{Command: command, Args: args})
	if err != nil {
		t.Fatalf("SpawnChild: %v", err)
	}
	if code, err := child.Wait(); err != nil || code != 0 {
		t.Fatalf("Wait = (%d, %v), want (0, nil)", code, err)
	}
	if err := child.Kill(); err != nil {
		t.Fatalf("Kill after Wait = %v, want nil", err)
	}
}

func TestChildLifetimeHoldsLoopUntilCleanupAfterKillError(t *testing.T) {
	runtime := goja.New()
	loop, err := goeventloop.New(goeventloop.WithAutoExit(true))
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := gojaeventloop.New(loop, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Bind(); err != nil {
		t.Fatal(err)
	}

	baseCtx, baseCancel := context.WithCancel(context.Background())
	loopCtx, loopCancel := context.WithCancel(context.Background())
	childDone := make(chan struct{})
	var childDoneOnce sync.Once
	finishChild := func() { childDoneOnce.Do(func() { close(childDone) }) }
	tree := &failingKillProcessTree{killStarted: make(chan struct{})}
	child := &ChildProcess{
		cmd:  &osexec.Cmd{Process: &os.Process{Pid: 42}},
		tree: tree,
		done: childDone,
	}
	loopDone := make(chan error, 1)
	go func() { loopDone <- loop.Run(loopCtx) }()
	t.Cleanup(func() {
		finishChild()
		baseCancel()
		loopCancel()
		if err := loop.Shutdown(context.Background()); err != nil && !errors.Is(err, goeventloop.ErrLoopTerminated) {
			t.Errorf("loop shutdown: %v", err)
		}
	})

	// The lifetime token is registered by the submitted task, so retry the
	// admission briefly: with auto-exit enabled the loop may commit to
	// termination before the first Submit arrives.
	submitLifetimeTask := func() {
		t.Helper()
		deadline := time.Now().Add(2 * time.Second)
		for {
			if err := loop.Submit(func() {
				trackChildProcessLifetime(baseCtx, runtime, adapter, child)
			}); err == nil {
				return
			} else if !errors.Is(err, goeventloop.ErrLoopTerminated) {
				t.Fatal(err)
			}
			if time.Now().After(deadline) {
				t.Fatal("loop terminated before lifetime task was admitted")
			}
			time.Sleep(time.Millisecond)
		}
	}
	submitLifetimeTask()
	baseCancel()
	select {
	case <-tree.killStarted:
	case <-time.After(time.Second):
		t.Fatal("child kill was not attempted after cancellation")
	}
	select {
	case err := <-loopDone:
		t.Fatalf("event loop exited before child cleanup completed: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	finishChild()
	select {
	case err := <-loopDone:
		if err != nil {
			t.Fatalf("event loop returned: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("event loop did not auto-exit after child cleanup completed")
	}
}

// TestRunExec_DrainTimeoutRecoversWithoutFailingTheCommand pins the drain
// recovery contract through the pipe factory: when the output pipes do not
// deliver EOF after the process tree terminates, the collector closes the
// readers at the drain timeout to recover, and the reads interrupted that
// way surface os.ErrClosed. Those induced errors are drain-recovery noise —
// they must not fail a command whose exit code was 0 — while the timeout
// itself must still be reported so a wedged child stays diagnosable. The
// pre-fix behavior failed the command with "read |0: file already closed"
// noise joined into the result (observed as spurious "git rm: (exec: ...)"
// failures in the pr-split gates under scheduler delay: the reader
// goroutines had not been scheduled before the timer fired).
func TestRunExec_DrainTimeoutRecoversWithoutFailingTheCommand(t *testing.T) {
	skipSlow(t)

	// Endpoints whose Read blocks until Close: exactly the shape of a
	// reader goroutine scheduled after the drain timer fired.
	stdoutReader := &blockingCommandPipe{release: make(chan struct{})}
	stderrReader := &blockingCommandPipe{release: make(chan struct{})}
	pipeNo := 0
	newPipe := func() (commandPipeEndpoint, commandPipeEndpoint, error) {
		pipeNo++
		switch pipeNo {
		case 1:
			return stdoutReader, &failingCommandPipe{}, nil
		case 2:
			return stderrReader, &failingCommandPipe{}, nil
		default:
			return &failingCommandPipe{}, &failingCommandPipe{}, nil
		}
	}

	command, args := immediateExitCommand()
	result := runExecWithPipeFactory(context.Background(), command, args, newPipe)

	code, ok := result["code"].(int)
	if !ok || code != 0 {
		t.Fatalf("runExec code = %v (%v), want 0 — the drain timeout must not fail a successful command", result["code"], result["message"])
	}
	if result["error"] != false {
		t.Fatalf("runExec error = %v, want false", result["error"])
	}
	message, _ := result["message"].(string)
	if !strings.Contains(message, "did not close after process-tree termination") {
		t.Fatalf("runExec message = %q, want the drain-timeout diagnostic", message)
	}
	if strings.Contains(message, "file already closed") {
		t.Fatalf("runExec message contains drain-recovery noise: %q", message)
	}
}

// blockingCommandPipe blocks in Read until closed, then reports the
// interruption exactly like an os.Pipe reader does.
type blockingCommandPipe struct {
	release chan struct{}
	once    sync.Once
}

func (p *blockingCommandPipe) Read([]byte) (int, error) {
	<-p.release
	return 0, os.ErrClosed
}

func (p *blockingCommandPipe) Write(data []byte) (int, error) { return len(data), nil }

func (p *blockingCommandPipe) Close() error {
	p.once.Do(func() { close(p.release) })
	return nil
}

// immediateExitCommand returns a command that exits 0 without writing.
func immediateExitCommand() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", "exit 0"}
	}
	return "true", nil
}
