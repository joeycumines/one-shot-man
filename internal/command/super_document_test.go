package command

import (
	"bytes"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSuperDocumentCommand(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)

	assert.Equal(t, "super-document", cmd.Name())
	assert.Contains(t, cmd.Description(), "TUI")
	assert.Contains(t, cmd.Description(), "document")
	assert.Contains(t, cmd.Usage(), "super-document")
}

func TestSuperDocumentCommand_SetupFlags(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)

	// Default values are set when flags are parsed, not at construction
	// Test that command is created with sensible defaults
	assert.NotNil(t, cmd.config)
	assert.Equal(t, "super-document", cmd.Name())
}

func TestSuperDocumentCommand_Execute_TestMode(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.store = "memory"

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{}, &stdout, &stderr)
	require.NoError(t, err)

	// In test mode, script should execute without entering interactive mode
	output := stdout.String()
	assert.Contains(t, output, "Super-Document")
}

func TestSuperDocumentCommand_Execute_WithSession(t *testing.T) {
	cfg := config.NewConfig()
	cmd := NewSuperDocumentCommand(cfg)
	cmd.testMode = true
	cmd.interactive = false
	cmd.session = "test-session-" + t.Name()
	cmd.store = "memory"

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{}, &stdout, &stderr)
	require.NoError(t, err)

	output := stdout.String()
	assert.Contains(t, output, "Super-Document")
}
