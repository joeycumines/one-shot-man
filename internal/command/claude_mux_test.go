package command

import (
	"bytes"
	"flag"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClaudeMuxCommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	assert.Equal(t, "claude-mux", cmd.Name())
	assert.Contains(t, cmd.Description(), "orchestration")
	assert.Contains(t, cmd.Usage(), "subcommand")
}

func TestClaudeMux_SetupFlags(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	cmd.SetupFlags(fs)

	// Default pool size should be 4.
	assert.Equal(t, 4, cmd.poolSize)

	// Parse custom pool-size.
	err := fs.Parse([]string{"-pool-size", "8"})
	require.NoError(t, err)
	assert.Equal(t, 8, cmd.poolSize)
}

func TestClaudeMux_NoArgs_ShowsHelp(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute(nil, &stdout, &stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "Usage: osm claude-mux")
}

func TestClaudeMux_UnknownSubcommand(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"bogus"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown subcommand")
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, stderr.String(), "bogus")
}

func TestClaudeMux_Status(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.poolSize = 6

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"status"}, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "claude-mux status")
	assert.Contains(t, out, "max-size:")
	assert.Contains(t, out, "6") // pool size
	assert.Contains(t, out, "rate-limit:")
	assert.Contains(t, out, "frequency-limit:")
	assert.Contains(t, out, "repeat-detection:")
	assert.Contains(t, out, "max-retries:")
	assert.Contains(t, out, "fail-closed")
}

func TestClaudeMux_Start(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)
	cmd.baseDir = t.TempDir()
	cmd.poolSize = 2

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"start"}, &stdout, &stderr)
	assert.NoError(t, err)

	out := stdout.String()
	assert.Contains(t, out, "[audit] instance registry:")
	assert.Contains(t, out, "[audit] pool started: max_size=2")
	assert.Contains(t, out, "[audit] session created: id=init-check state=Active")
	assert.Contains(t, out, "[audit] validation: event_type=Text action=none")
	assert.Contains(t, out, "[audit] session shutdown:")
	assert.Contains(t, out, "[audit] pool stats:")
	assert.Contains(t, out, "infrastructure validated successfully")
}

func TestClaudeMux_Stop(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"stop"}, &stdout, &stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), "no running instances")
}

func TestClaudeMux_Submit(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"submit", "fix", "the", "bug"}, &stdout, &stderr)
	assert.NoError(t, err)
	assert.Contains(t, stdout.String(), `"fix the bug"`)
	assert.Contains(t, stdout.String(), "[audit] task received:")
}

func TestClaudeMux_Submit_NoArgs(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"submit"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task description required")
}

func TestClaudeMux_Submit_EmptyTask(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewClaudeMuxCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"submit", "  ", " "}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}
