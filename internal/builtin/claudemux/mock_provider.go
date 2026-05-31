package claudemux

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

// MockClaudeProvider spawns the mockclaude test binary via NDJSON protocol mode.
// It is intended for integration tests that exercise the protocol mode pipeline
// without requiring a real Claude Code installation.
type MockClaudeProvider struct {
	// Path to the mockclaude binary. If empty, the binary is built on first use
	// from internal/cmd/mockclaude/ using "go build" and cached for subsequent spawns.
	Path string
	// ProcessingMs is the simulated processing delay passed via MOCK_PROCESSING_MS env var.
	ProcessingMs int
}

var (
	mockBinOnce sync.Once
	mockBinPath string
	mockBinErr  error
)

// mockBinaryPath builds the mockclaude binary once and caches the path.
func mockBinaryPath() (string, error) {
	mockBinOnce.Do(func() {
		if p, err := exec.LookPath("mockclaude"); err == nil {
			mockBinPath = p
			return
		}

		_, thisFile, _, ok := runtime.Caller(0)
		if !ok {
			mockBinErr = fmt.Errorf("claudemux: cannot locate source file for mock binary")
			return
		}

		mockBinPath, mockBinErr = buildMockBinary(thisFile)
	})
	return mockBinPath, mockBinErr
}

func buildMockBinary(sourceFile string) (string, error) {
	pkgDir := filepath.Dir(sourceFile)
	moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(pkgDir)))

	tmpDir, err := os.MkdirTemp("", "mockclaude-*")
	if err != nil {
		return "", fmt.Errorf("claudemux: temp dir for mock binary: %w", err)
	}

	binPath := filepath.Join(tmpDir, "mockclaude")
	cmd := exec.Command("go", "build", "-o", binPath, "./internal/cmd/mockclaude/")
	cmd.Dir = moduleRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("claudemux: build mock binary: %w\n%s", err, out)
	}

	return binPath, nil
}

func (p *MockClaudeProvider) Name() string { return "mock-claude" }

func (p *MockClaudeProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		MCP:       true,
		Streaming: true,
		MultiTurn: true,
		ModelNav:  false,
	}
}

func (p *MockClaudeProvider) Spawn(ctx context.Context, opts SpawnOpts) (AgentHandle, error) {
	bin, err := p.resolveCommand()
	if err != nil {
		return nil, err
	}

	ec := exec.CommandContext(ctx, bin)
	if opts.Dir != "" {
		ec.Dir = opts.Dir
	}

	env := make(map[string]string, len(opts.Env)+1)
	for k, v := range opts.Env {
		env[k] = v
	}
	if p.ProcessingMs > 0 {
		env["MOCK_PROCESSING_MS"] = fmt.Sprintf("%d", p.ProcessingMs)
	}
	if len(env) > 0 {
		ec.Env = append(ec.Environ(), envSlice(env)...)
	}

	h, err := newProtocolHandle(ec)
	if err != nil {
		return nil, fmt.Errorf("claudemux: mock provider: %w", err)
	}

	if err := ec.Start(); err != nil {
		h.Close()
		return nil, fmt.Errorf("claudemux: mock provider start: %w", err)
	}

	return h, nil
}

// Compile-time interface check.
var _ Provider = (*MockClaudeProvider)(nil)

func (p *MockClaudeProvider) resolveCommand() (string, error) {
	if p.Path != "" {
		return p.Path, nil
	}
	return mockBinaryPath()
}

func envSlice(env map[string]string) []string {
	s := make([]string, 0, len(env))
	for k, v := range env {
		s = append(s, k+"="+v)
	}
	return s
}
