package exec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// The read and the first close use a real OS pipe. Only the close result is
// faulted, after the child has closed its writer and the reader reached EOF.
type execFailureCloseReader struct {
	commandPipeEndpoint
	closeErr error
}

func (r *execFailureCloseReader) Close() error {
	return errors.Join(r.commandPipeEndpoint.Close(), r.closeErr)
}

func TestRunExec_ExecutionFailureIndependentOfExitStatus(t *testing.T) {
	skipSlow(t)
	for _, code := range []int{0, 1, 7} {
		for _, closeFailure := range []bool{false, true} {
			t.Run(fmt.Sprintf("exit_%d/close_failure_%t", code, closeFailure), func(t *testing.T) {
				closeErr := errors.New("injected output close after EOF")
				pipes := 0
				pipeFactory := func() (commandPipeEndpoint, commandPipeEndpoint, error) {
					reader, writer, err := newCommandPipe()
					pipes++
					if err == nil && pipes == 1 && closeFailure {
						reader = &execFailureCloseReader{commandPipeEndpoint: reader, closeErr: closeErr}
					}
					return reader, writer, err
				}
				command, args := exitCodeCommand(code)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				result := runExecCommand(ctx, SpawnConfig{Command: command, Args: args}, pipeFactory, newProcessTree, outputEOFCeiling)
				if result["code"] != code || result["truncated"] != false {
					t.Fatalf("observed exit or EOF changed: %v", result)
				}
				if result["error"] != (code != 0 || closeFailure) {
					t.Fatalf("error classification changed: %v", result)
				}
				flag, present := result["execFailed"]
				if closeFailure {
					if !present || flag != true || !strings.Contains(result["message"].(string), closeErr.Error()) {
						t.Fatalf("post-EOF execution failure was lost: %v", result)
					}
				} else if present {
					t.Fatalf("ordinary command gained execution failure metadata: %v", result)
				}
				if _, present := result["treeCleanupError"]; present {
					t.Fatalf("pipe-close result was presented as tree cleanup failure: %v", result)
				}
			})
		}
	}
}
