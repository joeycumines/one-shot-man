//go:build unix

package bouncelogo

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBouncingLogo_Smoke(t *testing.T) {
	binaryPath := buildTestBinary(t)
	root := testutil.RepoRootFromWD()
	scriptPath := filepath.Join(root, "scripts", "example-15-bouncing-logo.js")

	// Use a minimal mock shell that exits immediately to keep the smoke fast.
	mockContent := []byte("#!/bin/bash\necho 'MockShellReady'\n")
	tmpDir := t.TempDir()
	tmpMock := filepath.Join(tmpDir, "mock_shell.sh")
	require.NoError(t, os.WriteFile(tmpMock, mockContent, 0o755))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "script", scriptPath, "--smoke", "--cmd", "/bin/bash", "--", tmpMock)
	cmd.Dir = root
	cmd.Env = []string{
		"OSM_SESSION=bouncelogo-smoke-test",
		"OSM_STORE=memory",
		"PATH=" + os.Getenv("PATH"),
	}

	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	output := outBuf.String()
	stderr := errBuf.String()
	require.NoError(t, err, "smoke run failed. stdout:\n%s\nstderr:\n%s", output, stderr)

	assert.Contains(t, output, "smoke: rendered content length=", "smoke should print rendered content length")
	// Extract and assert positive length.
	lenIdx := strings.Index(output, "smoke: rendered content length=")
	require.GreaterOrEqual(t, lenIdx, 0)
	lenLine := strings.TrimSpace(output[lenIdx:])
	if nl := strings.Index(lenLine, "\n"); nl >= 0 {
		lenLine = lenLine[:nl]
	}
	var renderedLen int
	_, scanErr := fmt.Sscanf(lenLine, "smoke: rendered content length=%d", &renderedLen)
	require.NoError(t, scanErr, "could not parse rendered content length from: %q", lenLine)
	assert.Positive(t, renderedLen, "rendered content length must be positive")

	assert.Contains(t, output, "smoke: bounceCount=", "smoke should print bounceCount")
	bounceIdx := strings.Index(output, "smoke: bounceCount=")
	require.GreaterOrEqual(t, bounceIdx, 0)
	bounceLine := strings.TrimSpace(output[bounceIdx:])
	if nl := strings.Index(bounceLine, "\n"); nl >= 0 {
		bounceLine = bounceLine[:nl]
	}
	var bounceCount int
	_, scanErr = fmt.Sscanf(bounceLine, "smoke: bounceCount=%d", &bounceCount)
	require.NoError(t, scanErr, "could not parse bounceCount from: %q", bounceLine)
	assert.Positive(t, bounceCount, "bounceCount must be positive")

	assert.Contains(t, output, "smoke: title=true", "rendered view should contain the dashboard title")
	assert.Contains(t, output, "smoke: running=true", "rendered view should contain RUNNING indicator")

	assert.NotContains(t, output+stderr, "panic", "no panic in output")
	assert.NotContains(t, output+stderr, "TypeError", "no TypeError in output")
	assert.NotContains(t, output+stderr, "[object Object]", "no object Object in output")
}
