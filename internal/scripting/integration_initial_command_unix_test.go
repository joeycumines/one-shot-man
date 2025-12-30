//go:build unix

package scripting

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
)

func TestInitialCommand_DefersPrompt(t *testing.T) {
	binaryPath := buildTestBinary(t)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	projectDir := filepath.Clean(filepath.Join(wd, "..", ".."))
	scriptPath := filepath.Join(projectDir, "scripts", "test-02-initial-command.js")

	env := newTestProcessEnv(t)
	defaultTimeout := 20 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath, "script", "-i", scriptPath),
		termtest.WithEnv(env),
		termtest.WithDefaultTimeout(defaultTimeout),
	)
	if err != nil {
		t.Fatalf("Failed to create termtest: %v", err)
	}
	defer cp.Close()

	expect := func(timeout time.Duration, since termtest.Snapshot, cond termtest.Condition, description string) error {
		subCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return cp.Expect(subCtx, since, cond, description)
	}

	snap := cp.Snapshot()

	// The initial command switches to 'target' mode; expect the switch message
	if err := expect(defaultTimeout, snap, termtest.Contains("Switched to mode: target"), "initial command ran"); err != nil {
		t.Fatalf("initial command did not run as expected: %v", err)
	}

	// After the initial command finishes, prompt should already be the target prompt
	if err := expect(defaultTimeout, snap, termtest.Contains("[target]> "), "target prompt"); err != nil {
		t.Fatalf("expected target prompt after initial command: %v", err)
	}
}
