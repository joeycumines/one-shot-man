package exec

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
)

// A finite child with ordinary pipes must retain its complete output when
// consumed after reap. Child status alone is not proof of a complete stream;
// the complementary read-error tests below require truncation after any reap.
func TestSpawnChild_ReapedChildStreamIsNotTruncated(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	command, args := "sh", []string{"-c", "printf 'ok\\n'; exit 0"}
	if goruntime.GOOS == "windows" {
		command, args = "cmd.exe", []string{"/C", "echo ok & exit /b 0"}
	}

	for i := range 25 {
		child, err := SpawnChild(context.Background(), SpawnConfig{Command: command, Args: args})
		if err != nil {
			t.Fatalf("iter %d: SpawnChild: %v", i, err)
		}

		code, waitErr := child.WaitContext(context.Background())
		if waitErr != nil || code != 0 {
			t.Fatalf("iter %d: Wait = (%d, %v), want (0, nil)", i, code, waitErr)
		}

		// Read stdout to the terminal chunk after the recorded exit.
		var out string
		for {
			data, done, readErr := child.ReadStdoutContext(context.Background())
			out += data
			if readErr != nil {
				if errors.Is(readErr, errOutputStreamCutShort) {
					t.Fatalf("iter %d: finite child output did not reach EOF (out=%q)", i, out)
				}
				t.Fatalf("iter %d: terminal read error = %v", i, readErr)
			}
			if done {
				break
			}
		}
		if child.stdout.cutShort() || child.stderr.cutShort() {
			t.Fatalf("iter %d: reaped exit-0 child queue marked truncated (out=%q)", i, out)
		}
		if strings.TrimSpace(out) != "ok" {
			t.Fatalf("iter %d: output=%q, want exactly one ok line", i, out)
		}
	}
}

// A prefix followed by EIO is incomplete at every lifecycle point. Neither
// ProcessState publication nor the final exit record can replace a missing EOF.
func TestPumpPipe_NonEOFErrorStaysTruncatedAcrossReap(t *testing.T) {
	for _, state := range []string{"running", "process state only", "recorded exit", "tree close failed"} {
		t.Run(state, func(t *testing.T) {
			child := &ChildProcess{stdout: newPipeQueue(0), done: make(chan struct{}), abort: make(chan struct{})}
			switch state {
			case "process state only":
				child.cmd = &osexec.Cmd{ProcessState: &os.ProcessState{}}
			case "recorded exit":
				child.recordExit(nil, nil)
				close(child.done)
			case "tree close failed":
				child.recordExit(nil, errors.New("tree close failed"))
				close(child.done)
			}
			child.pumpPipe(&prefixThenEIOReader{prefix: []byte("prefix")}, &child.stdout)
			if data, done, err := child.ReadStdoutContext(context.Background()); data != "prefix" || done || err != nil {
				t.Fatalf("prefix=(%q,%v,%v)", data, done, err)
			}
			if _, done, err := child.ReadStdoutContext(context.Background()); !done || !errors.Is(err, errOutputStreamCutShort) {
				t.Fatalf("terminal=(%v,%v), want incomplete capture", done, err)
			}
		})
	}
}

func TestPumpPipe_ReadErrorConcurrentWithProcessReap(t *testing.T) {
	skipSlow(t)
	command, args := immediateExitCommand()
	for i := range 25 {
		cmd := osexec.Command(command, args...)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		child := &ChildProcess{cmd: cmd, stdout: newPipeQueue(0), abort: make(chan struct{})}
		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); <-start; child.recordExit(cmd.Wait(), nil) }()
		go func() {
			defer wg.Done()
			<-start
			child.pumpPipe(&prefixThenEIOReader{prefix: []byte("prefix")}, &child.stdout)
		}()
		close(start)
		wg.Wait()
		if !child.stdout.cutShort() {
			t.Fatalf("iteration %d: read failure became a complete stream", i)
		}
	}
}
