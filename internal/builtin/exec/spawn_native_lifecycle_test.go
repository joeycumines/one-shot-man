package exec

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

const outputLifecycleHelperEnv = "OSM_EXEC_OUTPUT_LIFECYCLE_HELPER"

func outputLifecyclePayload() string { return strings.Repeat("0123456789abcdef", 512) }

// TestMain dispatches here before the testing framework can add PASS output.
// The marker is written only after every payload byte has reached stdout.
func runOutputLifecycleHelper(mode string) {
	payload := outputLifecyclePayload()
	if mode == "unicode" {
		payload = "🧪 café 世界\n"
	} else if mode == "ascii" {
		payload = "0123456789abcdef"
	}
	if n, err := io.WriteString(os.Stdout, payload); err != nil || n != len(payload) {
		os.Exit(91)
	}
	if _, err := io.WriteString(os.Stderr, "ready"); err != nil {
		os.Exit(92)
	}
	if mode == "wait" {
		var b [1]byte
		_, _ = os.Stdin.Read(b[:])
	}
	os.Exit(0)
}

func spawnOutputLifecycleHelper(t *testing.T, ctx context.Context, mode string, capBytes int64) *ChildProcess {
	t.Helper()
	child, err := SpawnChild(ctx, SpawnConfig{
		Command: os.Args[0],
		Env: map[string]string{
			outputLifecycleHelperEnv: mode,
			"GORACE":                 "atexit_sleep_ms=0",
		},
		MaxBufferedOutputBytes: capBytes,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := child.Kill(); err != nil {
			t.Errorf("cleanup kill: %v", err)
		}
		waitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := child.WaitContext(waitCtx); errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("cleanup wait: %v", err)
		}
		if err := child.CloseStdinContext(context.Background()); err != nil && !errors.Is(err, os.ErrClosed) {
			t.Errorf("cleanup stdin: %v", err)
		}
	})
	return child
}

func awaitOutputLifecycleReady(t *testing.T, ctx context.Context, child *ChildProcess) {
	t.Helper()
	var marker strings.Builder
	for marker.Len() < len("ready") {
		data, done, err := child.ReadStderrContext(ctx)
		if err != nil || done {
			t.Fatalf("helper readiness: data=%q done=%v err=%v", marker.String(), done, err)
		}
		marker.WriteString(data)
	}
	if marker.String() != "ready" {
		t.Fatalf("helper marker=%q, want ready", marker.String())
	}
	// The kernel can hold the entire payload before stdout's pump runs.
	// Observe actual queue admission before an abort so its retained prefix
	// is established independently of scheduling during the shutdown grace.
	for {
		child.stdout.mu.Lock()
		retained, changed := child.stdout.bytes, child.stdout.notify
		child.stdout.mu.Unlock()
		if retained != 0 {
			break
		}
		select {
		case <-changed:
		case <-ctx.Done():
			t.Fatal("helper output never reached its queue")
		}
	}
}

func readLifecycleOutput(t *testing.T, ctx context.Context, child *ChildProcess) (string, error) {
	t.Helper()
	var output strings.Builder
	for {
		data, done, err := child.ReadStdoutContext(ctx)
		output.WriteString(data)
		if done {
			return output.String(), err
		}
		if err != nil {
			t.Fatalf("stdout read before terminal result: %v", err)
		}
	}
}

func TestSpawnChild_NativePostReapDrainLifecycle(t *testing.T) {
	skipSlow(t)
	for _, action := range []string{"drain", "kill", "cancel"} {
		t.Run(action, func(t *testing.T) {
			childCtx, cancelChild := context.WithCancel(context.Background())
			defer cancelChild()
			waitCtx, cancelWait := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancelWait()
			const capBytes = 4096
			child := spawnOutputLifecycleHelper(t, childCtx, "exit", capBytes)
			awaitOutputLifecycleReady(t, waitCtx, child)
			// No stdout read has happened. The finite payload is twice the
			// cap, so the pump cannot have retained it all or recorded EOF.
			if code, err := child.WaitContext(waitCtx); code != 0 || err != nil {
				t.Fatalf("wait before drain=(%d, %v), want (0, nil)", code, err)
			}
			switch action {
			case "kill":
				if err := child.Kill(); err != nil {
					t.Fatal(err)
				}
			case "cancel":
				// Cancel strictly after Wait returned to prove the raw Go
				// spawn still observes its context after process exit.
				cancelChild()
			}
			if action != "drain" {
				select {
				case <-child.settled:
				case <-waitCtx.Done():
					t.Fatal("post-reap abort did not settle the unread queue before the 30s drain ceiling")
				}
			}
			output, endErr := readLifecycleOutput(t, waitCtx, child)
			if action == "drain" {
				if output != outputLifecyclePayload() || endErr != nil {
					t.Fatalf("late drain captured %d/%d bytes, terminal error=%v", len(output), len(outputLifecyclePayload()), endErr)
				}
				if err := child.Kill(); err != nil {
					t.Fatal(err)
				}
			} else if len(output) == 0 || len(output) > capBytes || !strings.HasPrefix(outputLifecyclePayload(), output) || !errors.Is(endErr, errOutputStreamCutShort) {
				t.Fatalf("aborted drain captured %d bytes, terminal error=%v; want retained prefix <=%d and truncation", len(output), endErr, capBytes)
			}
			// Aborting a complete stream cannot change clean EOF, and a
			// truncated stream cannot turn clean on a repeated terminal read.
			if data, done, err := child.ReadStdoutContext(waitCtx); data != "" || !done || !errors.Is(err, endErr) {
				t.Fatalf("repeated terminal read=(%q, %v, %v), want empty, true, %v", data, done, err, endErr)
			}
			if code, err := child.WaitContext(waitCtx); code != 0 || err != nil {
				t.Fatalf("abort changed recorded exit=(%d, %v)", code, err)
			}
		})
	}
}

func TestSpawnChild_NativeKilledUnreadOutputReportsTruncation(t *testing.T) {
	skipSlow(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := spawnOutputLifecycleHelper(t, ctx, "wait", 4096)
	awaitOutputLifecycleReady(t, ctx, child)
	if err := child.Kill(); err != nil {
		t.Fatal(err)
	}
	if code, err := child.WaitContext(ctx); code == 0 || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("killed helper wait=(%d, %v)", code, err)
	}
	select {
	case <-child.settled:
	case <-ctx.Done():
		t.Fatal("killed helper drain did not settle")
	}
	output, err := readLifecycleOutput(t, ctx, child)
	if len(output) == 0 || len(output) > 4096 || !strings.HasPrefix(outputLifecyclePayload(), output) || !errors.Is(err, errOutputStreamCutShort) {
		t.Fatalf("killed helper captured %d bytes, terminal error=%v; want retained prefix and truncation", len(output), err)
	}
}
