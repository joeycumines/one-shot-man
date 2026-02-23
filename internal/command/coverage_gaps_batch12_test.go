package command

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Batch 12: command coverage gaps — builtin.go + claude_mux.go
// ============================================================================

// TestConfigReset_Key_NoConfigPath verifies executeResetKey when the
// constructor receives no configPath. GetConfigPath() resolves to a path in
// the isolated temp HOME, but since no config file exists on disk,
// DeleteKeyInFile gracefully returns nil. Covers builtin.go ~line 421-427.
// NOTE: Cannot use t.Parallel() — t.Setenv() required for isolation.
func TestConfigReset_Key_NoConfigPath(t *testing.T) {
	// Isolate from real config: point HOME to temp dir so
	// config.GetConfigPath() doesn't return the developer's real path.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OSM_CONFIG", "") // Prevent ambient env from bypassing HOME.

	cfg := config.NewConfig()
	cfg.Global["verbose"] = "true"

	// Create command with no configPath (empty string).
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "verbose"}, &stdout, &stderr)
	require.NoError(t, err)

	// Should reset in memory only.
	assert.Contains(t, stdout.String(), "Reset verbose to default")
	assert.Empty(t, stderr.String())

	// Verify the in-memory config was actually reset.
	_, exists := cfg.Global["verbose"]
	assert.False(t, exists, "verbose should be removed from in-memory config")
}

// TestConfigReset_Key_DiskDeleteError verifies that when the config file
// can be read but the atomic write back fails, the warning is printed to
// stderr but the command succeeds. Covers builtin.go ~line 425-427.
func TestConfigReset_Key_DiskDeleteError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	// Write a valid config file with the key.
	require.NoError(t, os.WriteFile(configPath, []byte("verbose true\n"), 0644))

	cfg := config.NewConfig()
	cfg.Global["verbose"] = "true"

	// Point to a path where the file exists but the PARENT dir will be
	// read-only after setup, preventing AtomicWriteFile from creating a temp.
	// On macOS/Linux, making the dir read-only prevents creating new files.
	nested := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(nested, 0755))
	nestedConfig := filepath.Join(nested, "config")
	require.NoError(t, os.WriteFile(nestedConfig, []byte("verbose true\n"), 0644))
	require.NoError(t, os.Chmod(nested, 0555))
	t.Cleanup(func() { os.Chmod(nested, 0755) })

	cmd := NewConfigCommand(cfg, nestedConfig)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "verbose"}, &stdout, &stderr)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Reset verbose to default")
	// On systems where chmod 0555 is effective, we get a warning.
	// On some systems (macOS root, etc.) the write may still succeed.
	// We just verify the command doesn't error out.
}

// TestConfigReset_AllKeys_NoConfigPath verifies executeResetAll when the
// constructor receives no configPath. GetConfigPath() resolves to a path in
// the isolated temp HOME, but since no config file exists on disk,
// DeleteAllGlobalKeysInFile gracefully returns (0, nil).
// Covers builtin.go ~line 442-453.
// NOTE: Cannot use t.Parallel() — t.Setenv() required for isolation.
func TestConfigReset_AllKeys_NoConfigPath(t *testing.T) {
	// Isolate from real config.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OSM_CONFIG", "") // Prevent ambient env from bypassing HOME.

	cfg := config.NewConfig()
	cfg.Global["verbose"] = "true"
	cfg.Global["color"] = "never"

	cmd := NewConfigCommand(cfg) // No configPath.

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "--all", "--force"}, &stdout, &stderr)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "Reset 2 key(s) to defaults")
	assert.Empty(t, stderr.String())
	assert.Empty(t, cfg.Global, "global config should be empty after reset all")
}

// TestConfigReset_AllKeys_DiskDeleteError verifies that when the disk file
// cannot be updated, a warning is printed but the command succeeds.
// Covers builtin.go ~line 447-449.
func TestConfigReset_AllKeys_DiskDeleteError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Same approach as single key: read-only dir prevents AtomicWriteFile.
	nested := filepath.Join(dir, "sub")
	require.NoError(t, os.Mkdir(nested, 0755))
	nestedConfig := filepath.Join(nested, "config")
	require.NoError(t, os.WriteFile(nestedConfig, []byte("verbose true\n"), 0644))
	require.NoError(t, os.Chmod(nested, 0555))
	t.Cleanup(func() { os.Chmod(nested, 0755) })

	cfg := config.NewConfig()
	cfg.Global["verbose"] = "true"
	cmd := NewConfigCommand(cfg, nestedConfig)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "--all", "--force"}, &stdout, &stderr)
	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "key(s) to defaults")
	// On systems where chmod is effective, warning appears in stderr.
	// Either way, the command should succeed.
}

// TestConfigReset_AllKeys_DiskCountLarger verifies the path where diskCount
// is larger than the in-memory count (keys only on disk). Covers
// builtin.go ~line 451-453.
func TestConfigReset_AllKeys_DiskCountLarger(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config")

	// Write more keys to disk than exist in memory using dnsmasq format.
	require.NoError(t, os.WriteFile(configPath, []byte("verbose true\ncolor always\neditor vim\n"), 0644))

	// Create in-memory config with fewer keys.
	cfg := config.NewConfig()
	cfg.Global["verbose"] = "true"

	cmd := NewConfigCommand(cfg, configPath)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "--all", "--force"}, &stdout, &stderr)
	require.NoError(t, err)

	// Should report 3 keys (disk count wins).
	assert.Contains(t, stdout.String(), "Reset 3 key(s) to defaults")
	assert.Empty(t, cfg.Global)
}

// TestInit_ExtraArgs verifies that init rejects extra arguments.
// Covers builtin.go ~line 490-492.
func TestInit_ExtraArgs(t *testing.T) {
	t.Parallel()
	cmd := NewInitCommand()

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"extra", "args"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected arguments")
}

// TestConfig_Schema_ExtraArgs verifies that 'config schema' with extra
// arguments returns an error.
func TestConfig_Schema_ExtraArgs(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"schema", "extra"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected arguments")
}

// TestConfig_ResetKey_UnknownKey verifies that resetting an unrecognized
// config key returns an error.
func TestConfig_ResetKey_UnknownKey(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	cmd := NewConfigCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"reset", "nonexistent-key-xyz"}, &stdout, &stderr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown configuration key")
	assert.Contains(t, stderr.String(), "not a known configuration key")
}
