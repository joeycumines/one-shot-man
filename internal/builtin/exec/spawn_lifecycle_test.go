package exec

import (
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
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
