package command

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// --- config-push tests ---

func TestSyncCommand_ConfigPushDisabled(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	// sync.config-sync not set (default false)
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-push"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for disabled config sync")
	}
	if !strings.Contains(err.Error(), "config sync disabled") {
		t.Fatalf("expected 'config sync disabled', got %q", err.Error())
	}
}

func TestSyncCommand_ConfigPushNoConfig(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(nil, t.TempDir())

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-push"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
	if !strings.Contains(err.Error(), "no configuration loaded") {
		t.Fatalf("expected 'no configuration loaded', got %q", err.Error())
	}
}

func TestSyncCommand_ConfigPushUnexpectedArgs(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-push", "extra"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for extra args")
	}
	if !errors.Is(err, ErrUnexpectedArguments) {
		t.Fatalf("expected ErrUnexpectedArguments, got %q", err.Error())
	}
}

func TestSyncCommand_ConfigPushNoKeys(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")
	cmd := NewSyncCommand(cfg)

	// Clear all global options — only sync.local-path is set, which is sensitive.
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-push"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-push failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "No shareable config keys") {
		t.Fatalf("expected 'No shareable config keys', got %q", stdout.String())
	}
}

func TestSyncCommand_ConfigPushWritesSharedConf(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "true")
	cfg.SetGlobalOption("prompt.template", "my-template")
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-push"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-push failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "Pushed") {
		t.Fatalf("expected push confirmation, got %q", output)
	}
	if !strings.Contains(output, "SHA:") {
		t.Fatalf("expected SHA in output, got %q", output)
	}

	// Verify the file was created.
	sharedPath := filepath.Join(root, "config", "shared.conf")
	data, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("shared.conf not created: %v", err)
	}

	content := string(data)
	if !strings.HasPrefix(content, "# osm-shared-config-version 1\n") {
		t.Fatalf("expected version header, got %q", content)
	}
	if !strings.Contains(content, "goal.autodiscovery true") {
		t.Fatalf("expected goal.autodiscovery key, got %q", content)
	}
	if !strings.Contains(content, "prompt.template my-template") {
		t.Fatalf("expected prompt.template key, got %q", content)
	}
}

func TestSyncCommand_ConfigPushExcludesSensitiveKeys(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")
	cfg.SetGlobalOption("sync.repository", "git@github.com:test/repo.git")
	cfg.SetGlobalOption("sync.auto-pull", "true")
	cfg.SetGlobalOption("sync.config-sha", "abc123")
	cfg.SetGlobalOption("log.file", "/var/log/osm.log")
	cfg.SetGlobalOption("session.max-age-days", "30")
	cfg.SetGlobalOption("goal.autodiscovery", "true") // shareable
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-push"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-push failed: %v", err)
	}

	sharedPath := filepath.Join(root, "config", "shared.conf")
	data, err := os.ReadFile(sharedPath)
	if err != nil {
		t.Fatalf("shared.conf not created: %v", err)
	}

	content := string(data)
	// Sensitive keys should NOT be present.
	if strings.Contains(content, "sync.repository") {
		t.Fatalf("sync.repository should be excluded, got %q", content)
	}
	if strings.Contains(content, "sync.auto-pull") {
		t.Fatalf("sync.auto-pull should be excluded, got %q", content)
	}
	if strings.Contains(content, "sync.config-sha") {
		t.Fatalf("sync.config-sha should be excluded, got %q", content)
	}
	if strings.Contains(content, "log.file") {
		t.Fatalf("log.file should be excluded, got %q", content)
	}
	if strings.Contains(content, "session.") {
		t.Fatalf("session.* should be excluded, got %q", content)
	}
	// Shareable key should be present.
	if !strings.Contains(content, "goal.autodiscovery true") {
		t.Fatalf("expected goal.autodiscovery, got %q", content)
	}
}

func TestSyncCommand_ConfigPushStoresSHA(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")
	cfg.SetGlobalOption("goal.paths", "/my/goals")
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-push"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-push failed: %v", err)
	}

	sha := cfg.GetString("sync.config-sha")
	if sha == "" {
		t.Fatal("expected sync.config-sha to be set after push")
	}
	if len(sha) != 64 { // SHA256 hex is 64 chars
		t.Fatalf("expected 64 char SHA, got %d: %q", len(sha), sha)
	}
}

func TestSyncCommand_ConfigPushKeysSorted(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")
	cfg.SetGlobalOption("zebra.key", "z")
	cfg.SetGlobalOption("alpha.key", "a")
	cfg.SetGlobalOption("middle.key", "m")
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	if err := cmd.Execute([]string{"config-push"}, &stdout, &stderr); err != nil {
		t.Fatalf("config-push failed: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(root, "config", "shared.conf"))
	lines := strings.Split(string(data), "\n")
	// Lines should be: version header, alpha.key, middle.key, zebra.key, trailing empty
	if len(lines) < 4 {
		t.Fatalf("expected at least 4 lines, got %d: %q", len(lines), string(data))
	}
	if !strings.HasPrefix(lines[1], "alpha.key") {
		t.Fatalf("expected alpha.key first, got %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "middle.key") {
		t.Fatalf("expected middle.key second, got %q", lines[2])
	}
	if !strings.HasPrefix(lines[3], "zebra.key") {
		t.Fatalf("expected zebra.key third, got %q", lines[3])
	}
}

// --- config-pull tests ---

func TestSyncCommand_ConfigPullNoConfig(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(nil, t.TempDir())

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
	if !strings.Contains(err.Error(), "no configuration loaded") {
		t.Fatalf("expected 'no configuration loaded', got %q", err.Error())
	}
}

func TestSyncCommand_ConfigPullDisabled(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	// sync.config-sync not set (default false)
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for disabled config sync")
	}
	if !strings.Contains(err.Error(), "config sync disabled") {
		t.Fatalf("expected 'config sync disabled', got %q", err.Error())
	}
}

func TestSyncCommand_ConfigPullNoSharedConf(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for missing shared.conf")
	}
	if !strings.Contains(err.Error(), "shared config not found") {
		t.Fatalf("expected 'shared config not found', got %q", err.Error())
	}
}

func TestSyncCommand_ConfigPullFirstTimeNoForce(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")

	// Create shared.conf.
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# osm-shared-config-version 1\ngoal.autodiscovery true\n"
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for first pull without --force")
	}
	if !strings.Contains(err.Error(), "unknown sync state") {
		t.Fatalf("expected 'unknown sync state', got %q", err.Error())
	}
}

func TestSyncCommand_ConfigPullFirstTimeWithForce(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# osm-shared-config-version 1\ngoal.autodiscovery true\nprompt.template shared-tmpl\n"
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull", "--force"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-pull --force failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "Applied 2 config keys") {
		t.Fatalf("expected 'Applied 2 config keys', got %q", output)
	}

	// Verify keys were applied.
	if v := cfg.GetString("goal.autodiscovery"); v != "true" {
		t.Fatalf("expected goal.autodiscovery=true, got %q", v)
	}
	if v := cfg.GetString("prompt.template"); v != "shared-tmpl" {
		t.Fatalf("expected prompt.template=shared-tmpl, got %q", v)
	}

	// SHA should be stored.
	sha := cfg.GetString("sync.config-sha")
	if sha == "" {
		t.Fatal("expected sync.config-sha to be set")
	}
}

func TestSyncCommand_ConfigPullAlreadyUpToDate(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# osm-shared-config-version 1\ngoal.autodiscovery true\n"
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Pre-set the SHA to match.
	cfg.SetGlobalOption("sync.config-sha", sha256Hex(content))

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-pull failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "already up to date") {
		t.Fatalf("expected 'already up to date', got %q", stdout.String())
	}
}

func TestSyncCommand_ConfigPullRemoteChanged(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Old content — SHA was stored against this.
	oldContent := "# osm-shared-config-version 1\ngoal.autodiscovery false\n"
	cfg.SetGlobalOption("sync.config-sha", sha256Hex(oldContent))

	// New content — remote changed.
	newContent := "# osm-shared-config-version 1\ngoal.autodiscovery true\nprompt.template updated\n"
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte(newContent), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-pull failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "Applied 2 config keys") {
		t.Fatalf("expected 'Applied 2 config keys', got %q", output)
	}

	// Verify updated values.
	if v := cfg.GetString("goal.autodiscovery"); v != "true" {
		t.Fatalf("expected goal.autodiscovery=true, got %q", v)
	}
	if v := cfg.GetString("prompt.template"); v != "updated" {
		t.Fatalf("expected prompt.template=updated, got %q", v)
	}

	// SHA should be updated to new content.
	if cfg.GetString("sync.config-sha") != sha256Hex(newContent) {
		t.Fatal("expected SHA to be updated to new content SHA")
	}
}

func TestSyncCommand_ConfigPullSkipsSensitiveKeys(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	// Shared config with a sensitive key that should be skipped.
	content := "# osm-shared-config-version 1\nsync.repository should-be-skipped\ngoal.autodiscovery true\n"
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull", "--force"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-pull failed: %v", err)
	}

	// sync.repository should NOT be applied (sensitive).
	if v := cfg.GetString("sync.repository"); v == "should-be-skipped" {
		t.Fatal("sensitive key sync.repository should NOT be applied from shared config")
	}

	// Non-sensitive key should be applied.
	if v := cfg.GetString("goal.autodiscovery"); v != "true" {
		t.Fatalf("expected goal.autodiscovery=true, got %q", v)
	}

	// Warning should appear.
	if !strings.Contains(stderr.String(), "skipping sensitive key") {
		t.Fatalf("expected warning about sensitive key, got stderr %q", stderr.String())
	}

	// Applied count should be 1 (only goal.autodiscovery), not 2.
	if !strings.Contains(stdout.String(), "Applied 1 config keys") {
		t.Fatalf("expected 'Applied 1 config keys', got %q", stdout.String())
	}
}

func TestSyncCommand_ConfigPullSchemaVersionTooNew(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# osm-shared-config-version 999\ngoal.autodiscovery true\n"
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull", "--force"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
	if !strings.Contains(err.Error(), "unsupported shared config version") {
		t.Fatalf("expected version error, got %q", err.Error())
	}
}

func TestSyncCommand_ConfigPullBadVersionHeader(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		content string
		errMsg  string
	}{
		{
			name:    "empty file",
			content: "",
			errMsg:  "empty shared config file",
		},
		{
			name:    "missing version header",
			content: "# some other comment\nkey value\n",
			errMsg:  "missing version header",
		},
		{
			name:    "no version number",
			content: "# osm-shared-config-version\n",
			errMsg:  "version header missing version number",
		},
		{
			name:    "invalid version number",
			content: "# osm-shared-config-version abc\n",
			errMsg:  "invalid version number",
		},
		{
			name:    "zero version",
			content: "# osm-shared-config-version 0\n",
			errMsg:  "invalid version number 0",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			cfgLocal := config.NewConfig()
			cfgLocal.SetGlobalOption("sync.local-path", dir)
			cfgLocal.SetGlobalOption("sync.config-sync", "true")
			cfgLocal.SetGlobalOption("sync.config-sha", "dummy")

			d := filepath.Join(dir, "config")
			if err := os.MkdirAll(d, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(d, "shared.conf"), []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}

			cmd := NewSyncCommand(cfgLocal)
			var stdout, stderr bytes.Buffer
			err := cmd.Execute([]string{"config-pull"}, &stdout, &stderr)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.errMsg) {
				t.Fatalf("expected error containing %q, got %q", tc.errMsg, err.Error())
			}
		})
	}
}

func TestSyncCommand_ConfigPullUnexpectedArg(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte("# osm-shared-config-version 1\n"), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull", "--bad"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for bad arg")
	}
	if !errors.Is(err, ErrUnexpectedArguments) {
		t.Fatalf("expected ErrUnexpectedArguments, got %q", err.Error())
	}
}

func TestSyncCommand_ConfigPullShortForceFlag(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# osm-shared-config-version 1\ngoal.autodiscovery true\n"
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull", "-f"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-pull -f failed: %v", err)
	}
	if !strings.Contains(stdout.String(), "Applied") {
		t.Fatalf("expected 'Applied', got %q", stdout.String())
	}
}

// --- Push+Pull round trip ---

func TestSyncCommand_ConfigPushPullRoundTrip(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "true")
	cfg.SetGlobalOption("prompt.template", "my-template")
	cfg.SetGlobalOption("hot-snippets.no-warning", "true")
	cmd := NewSyncCommand(cfg)

	// Push.
	var pOut, pErr bytes.Buffer
	if err := cmd.Execute([]string{"config-push"}, &pOut, &pErr); err != nil {
		t.Fatalf("push failed: %v", err)
	}
	pushSHA := cfg.GetString("sync.config-sha")
	if pushSHA == "" {
		t.Fatal("expected SHA after push")
	}

	// Simulate a different machine: new config with the same SHA.
	cfg2 := config.NewConfig()
	cfg2.SetGlobalOption("sync.local-path", root)
	cfg2.SetGlobalOption("sync.config-sync", "true")
	cfg2.SetGlobalOption("sync.config-sha", "different-sha-from-old-version")
	cmd2 := NewSyncCommand(cfg2)

	// Pull — should apply because SHA differs.
	var rOut, rErr bytes.Buffer
	if err := cmd2.Execute([]string{"config-pull"}, &rOut, &rErr); err != nil {
		t.Fatalf("pull failed: %v\nstderr: %s", err, rErr.String())
	}

	if cfg2.GetString("goal.autodiscovery") != "true" {
		t.Fatal("expected goal.autodiscovery=true after pull")
	}
	if cfg2.GetString("prompt.template") != "my-template" {
		t.Fatal("expected prompt.template=my-template after pull")
	}
	if cfg2.GetString("hot-snippets.no-warning") != "true" {
		t.Fatal("expected hot-snippets.no-warning=true after pull")
	}

	// Pull again — should be no-op (SHA matches).
	var r2Out, r2Err bytes.Buffer
	if err := cmd2.Execute([]string{"config-pull"}, &r2Out, &r2Err); err != nil {
		t.Fatalf("second pull failed: %v", err)
	}
	if !strings.Contains(r2Out.String(), "already up to date") {
		t.Fatalf("expected 'already up to date' on second pull, got %q", r2Out.String())
	}
}

// --- parseSharedConfig unit tests ---

func TestParseSharedConfig_Valid(t *testing.T) {
	t.Parallel()
	content := "# osm-shared-config-version 1\ngoal.autodiscovery true\nprompt.template default\n"
	version, keys, err := parseSharedConfig(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0].key != "goal.autodiscovery" || keys[0].value != "true" {
		t.Fatalf("expected goal.autodiscovery=true, got %q=%q", keys[0].key, keys[0].value)
	}
}

func TestParseSharedConfig_WithComments(t *testing.T) {
	t.Parallel()
	content := "# osm-shared-config-version 1\n# comment line\n\ngoal.paths /my/path\n# another comment\n"
	version, keys, err := parseSharedConfig(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].key != "goal.paths" || keys[0].value != "/my/path" {
		t.Fatalf("expected goal.paths=/my/path, got %q=%q", keys[0].key, keys[0].value)
	}
}

func TestParseSharedConfig_KeyWithoutValue(t *testing.T) {
	t.Parallel()
	content := "# osm-shared-config-version 1\nsomekey\n"
	version, keys, err := parseSharedConfig(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].key != "somekey" || keys[0].value != "" {
		t.Fatalf("expected somekey=<empty>, got %q=%q", keys[0].key, keys[0].value)
	}
}

func TestParseSharedConfig_ValueWithSpaces(t *testing.T) {
	t.Parallel()
	content := "# osm-shared-config-version 1\nprompt.template this is a long value with spaces\n"
	_, keys, err := parseSharedConfig(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].value != "this is a long value with spaces" {
		t.Fatalf("expected full value, got %q", keys[0].value)
	}
}

// --- isSensitiveKey unit tests ---

func TestIsSensitiveKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		key  string
		want bool
	}{
		{"sync.repository", true},
		{"sync.auto-pull", true},
		{"sync.config-sha", true},
		{"sync.local-path", true},
		{"log.file", true},
		{"session.max-age-days", true},
		{"session.cleanup", true},
		{"goal.autodiscovery", false},
		{"prompt.template", false},
		{"hot-snippets.no-warning", false},
		{"log.level", false},
		{"log.max-size-mb", false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.key, func(t *testing.T) {
			t.Parallel()
			got := isSensitiveKey(tc.key)
			if got != tc.want {
				t.Fatalf("isSensitiveKey(%q) = %v, want %v", tc.key, got, tc.want)
			}
		})
	}
}

// --- sha256Hex unit test ---

func TestSha256Hex(t *testing.T) {
	t.Parallel()
	// Known value: SHA256 of "hello"
	got := sha256Hex("hello")
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if got != want {
		t.Fatalf("sha256Hex(\"hello\") = %q, want %q", got, want)
	}
}

// --- parseVersionHeader unit tests ---

func TestParseVersionHeader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		line    string
		want    int
		wantErr string
	}{
		{"# osm-shared-config-version 1", 1, ""},
		{"# osm-shared-config-version 2", 2, ""},
		{"# osm-shared-config-version 42", 42, ""},
		{"# wrong prefix", 0, "missing version header"},
		{"# osm-shared-config-version", 0, "version header missing version number"},
		{"# osm-shared-config-version abc", 0, "invalid version number"},
		{"# osm-shared-config-version 0", 0, "invalid version number 0"},
		{"# osm-shared-config-version -1", 0, "invalid version number -1"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.line, func(t *testing.T) {
			t.Parallel()
			got, err := parseVersionHeader(tc.line)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("parseVersionHeader(%q) = %d, want %d", tc.line, got, tc.want)
			}
		})
	}
}

// --- Usage text update test ---

func TestSyncCommand_NoSubcommandShowsConfigSubcommands(t *testing.T) {
	t.Parallel()
	cmd := NewSyncCommand(nil, t.TempDir())

	var stdout, stderr bytes.Buffer
	_ = cmd.Execute(nil, &stdout, &stderr)

	usage := stderr.String()
	if !strings.Contains(usage, "config-push") {
		t.Fatalf("expected 'config-push' in usage, got %q", usage)
	}
	if !strings.Contains(usage, "config-pull") {
		t.Fatalf("expected 'config-pull' in usage, got %q", usage)
	}
}

// --- New enhancement tests ---

func TestSyncCommand_ConfigPullDryRun(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "old-value")

	// Set a stored SHA that differs from what we'll write, so pull proceeds.
	cfg.SetGlobalOption("sync.config-sha", "stale-sha")

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# osm-shared-config-version 1\ngoal.autodiscovery new-value\nprompt.template added-key\n"
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull", "--dry-run"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-pull --dry-run failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "Dry run: no changes applied") {
		t.Fatalf("expected dry-run message, got %q", output)
	}
	if !strings.Contains(output, "Config diff:") {
		t.Fatalf("expected diff summary, got %q", output)
	}

	// Values must NOT have been applied.
	if v := cfg.GetString("goal.autodiscovery"); v != "old-value" {
		t.Fatalf("expected goal.autodiscovery=old-value (unchanged), got %q", v)
	}
	if v := cfg.GetString("prompt.template"); v != "" {
		t.Fatalf("expected prompt.template to be unset (dry-run), got %q", v)
	}
}

func TestSyncCommand_ConfigPullConflictSummary(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")

	// Local state: one key matches remote, one differs, one is absent in remote.
	cfg.SetGlobalOption("goal.autodiscovery", "true")   // same as remote → unchanged
	cfg.SetGlobalOption("prompt.template", "old-value") // differs → updated
	// "hot-snippets.no-warning" not set locally → added

	// Set stored SHA to something different so pull proceeds.
	cfg.SetGlobalOption("sync.config-sha", "old-sha")

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# osm-shared-config-version 1\ngoal.autodiscovery true\nhot-snippets.no-warning true\nprompt.template new-value\n"
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-pull failed: %v\nstderr: %s", err, stderr.String())
	}

	output := stdout.String()
	// Should contain the summary line with correct counts.
	if !strings.Contains(output, "1 added") {
		t.Fatalf("expected '1 added' in summary, got %q", output)
	}
	if !strings.Contains(output, "1 updated") {
		t.Fatalf("expected '1 updated' in summary, got %q", output)
	}
	if !strings.Contains(output, "1 unchanged") {
		t.Fatalf("expected '1 unchanged' in summary, got %q", output)
	}

	// Verify changes were actually applied.
	if !strings.Contains(output, "Applied 3 config keys") {
		t.Fatalf("expected 'Applied 3 config keys', got %q", output)
	}
}

func TestSyncCommand_ConfigPushAtomicWrite(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")
	cfg.SetGlobalOption("goal.autodiscovery", "true")
	cmd := NewSyncCommand(cfg)

	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-push"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-push failed: %v\nstderr: %s", err, stderr.String())
	}

	// Verify the final file exists.
	sharedPath := filepath.Join(root, "config", "shared.conf")
	if _, err := os.Stat(sharedPath); err != nil {
		t.Fatalf("shared.conf not created: %v", err)
	}

	// Verify no .tmp file remains.
	tmpPath := sharedPath + ".tmp"
	if _, err := os.Stat(tmpPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected .tmp file to be cleaned up, but it exists")
	}
}

func TestSyncCommand_ConfigLockAcquisition(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Acquire lock.
	unlock, err := syncConfigLock(root)
	if err != nil {
		t.Fatalf("first lock acquisition failed: %v", err)
	}

	// Verify lock file exists.
	lockPath := syncConfigLockPath(root)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file does not exist: %v", err)
	}

	// Verify lock file contains PID and timestamp.
	data, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatalf("reading lock file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "pid=") {
		t.Fatalf("lock file missing pid, got %q", content)
	}
	if !strings.Contains(content, "time=") {
		t.Fatalf("lock file missing time, got %q", content)
	}

	// Second acquisition must fail.
	_, err = syncConfigLock(root)
	if err == nil {
		t.Fatal("expected error on second lock acquisition")
	}
	if !strings.Contains(err.Error(), "another sync operation is in progress") {
		t.Fatalf("expected 'another sync operation' error, got %q", err.Error())
	}

	// Release first lock.
	unlock()
}

func TestSyncCommand_ConfigLockCleanup(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	unlock, err := syncConfigLock(root)
	if err != nil {
		t.Fatalf("lock acquisition failed: %v", err)
	}

	lockPath := syncConfigLockPath(root)
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file does not exist after acquisition: %v", err)
	}

	// Release the lock.
	unlock()

	// Lock file should be gone.
	if _, err := os.Stat(lockPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock file should be removed after unlock, stat returned: %v", err)
	}

	// Should be able to acquire again.
	unlock2, err := syncConfigLock(root)
	if err != nil {
		t.Fatalf("re-acquisition after cleanup failed: %v", err)
	}
	unlock2()
}

func TestSyncCommand_StaleLockRecovery_DeadPID(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Create a lock file with a PID that is almost certainly not alive
	// (999999999 exceeds typical PID ranges). On Unix, signal-0 detects
	// this; on Windows processAlive is conservative (returns true), so we
	// also set an old timestamp to ensure the age check triggers.
	lockPath := syncConfigLockPath(root)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		t.Fatalf("creating lock dir: %v", err)
	}
	oldTime := time.Now().Add(-11 * time.Minute).UTC().Format(time.RFC3339)
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("pid=999999999\ntime=%s\n", oldTime)), 0644); err != nil {
		t.Fatalf("writing stale lock: %v", err)
	}

	// syncConfigLock should detect the stale lock and recover.
	unlock, err := syncConfigLock(root)
	if err != nil {
		t.Fatalf("expected stale lock recovery, got error: %v", err)
	}
	unlock()
}

func TestSyncCommand_StaleLockRecovery_AgedOut(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Create a lock file with the current PID (alive) but an old timestamp.
	lockPath := syncConfigLockPath(root)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		t.Fatalf("creating lock dir: %v", err)
	}
	oldTime := time.Now().Add(-11 * time.Minute).UTC().Format(time.RFC3339)
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("pid=%d\ntime=%s\n", os.Getpid(), oldTime)), 0644); err != nil {
		t.Fatalf("writing aged lock: %v", err)
	}

	// syncConfigLock should detect the aged lock and recover.
	unlock, err := syncConfigLock(root)
	if err != nil {
		t.Fatalf("expected aged lock recovery, got error: %v", err)
	}
	unlock()
}

func TestSyncCommand_ActiveLockNotStale(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	// Create a lock file with the current (alive) PID and fresh timestamp.
	lockPath := syncConfigLockPath(root)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		t.Fatalf("creating lock dir: %v", err)
	}
	freshTime := time.Now().UTC().Format(time.RFC3339)
	if err := os.WriteFile(lockPath, []byte(fmt.Sprintf("pid=%d\ntime=%s\n", os.Getpid(), freshTime)), 0644); err != nil {
		t.Fatalf("writing active lock: %v", err)
	}

	// syncConfigLock should NOT remove an active lock.
	_, err := syncConfigLock(root)
	if err == nil {
		t.Fatal("expected error for active lock, got nil")
	}
	if !strings.Contains(err.Error(), "another sync operation is in progress") {
		t.Fatalf("expected 'another sync operation' error, got %q", err.Error())
	}
}

func TestSyncCommand_ConfigPushGitignoreWarning(t *testing.T) {
	t.Parallel()

	// checkGitignored depends on git. Test the function directly.
	// In a non-git directory, checkGitignored should return false (git fails).
	root := t.TempDir()
	if checkGitignored(root, filepath.Join(root, "somefile")) {
		t.Fatal("expected checkGitignored=false in non-git directory")
	}

	// Set up a git repo with a .gitignore that ignores shared.conf.
	gitDir := t.TempDir()
	// Check if git is available.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available, skipping gitignore test")
	}

	// git init
	gitInit := exec.Command("git", "init", gitDir)
	if out, err := gitInit.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Create .gitignore that ignores shared.conf.
	if err := os.WriteFile(filepath.Join(gitDir, ".gitignore"), []byte("shared.conf\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sharedPath := filepath.Join(gitDir, "shared.conf")
	if err := os.WriteFile(sharedPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	if !checkGitignored(gitDir, sharedPath) {
		t.Fatal("expected checkGitignored=true for gitignored file")
	}

	// A file not in the gitignore should return false.
	otherPath := filepath.Join(gitDir, "other.txt")
	if err := os.WriteFile(otherPath, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if checkGitignored(gitDir, otherPath) {
		t.Fatal("expected checkGitignored=false for non-ignored file")
	}
}

func TestComputeConfigDiff(t *testing.T) {
	t.Parallel()

	local := map[string]string{
		"goal.autodiscovery":      "true",
		"prompt.template":         "old",
		"hot-snippets.no-warning": "true",
	}
	remote := []configKeyValue{
		{key: "goal.autodiscovery", value: "true"},       // unchanged
		{key: "prompt.template", value: "new"},           // updated
		{key: "editor.font-size", value: "14"},           // added
		{key: "sync.repository", value: "git@host:repo"}, // sensitive — should be excluded
	}

	summary := computeConfigDiff(local, remote)

	if len(summary.unchanged) != 1 || summary.unchanged[0] != "goal.autodiscovery" {
		t.Fatalf("expected 1 unchanged (goal.autodiscovery), got %v", summary.unchanged)
	}
	if len(summary.updated) != 1 || summary.updated[0].key != "prompt.template" {
		t.Fatalf("expected 1 updated (prompt.template), got %v", summary.updated)
	}
	if len(summary.added) != 1 || summary.added[0].key != "editor.font-size" {
		t.Fatalf("expected 1 added (editor.font-size), got %v", summary.added)
	}
}

func TestSyncCommand_ConfigPullEmptySyncRoot(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	// Point to a path that does NOT exist.
	nonexistent := filepath.Join(t.TempDir(), "does-not-exist")
	cfg.SetGlobalOption("sync.local-path", nonexistent)
	cfg.SetGlobalOption("sync.config-sync", "true")
	cfg.SetGlobalOption("sync.config-sha", "dummy")

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull"}, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for nonexistent sync root")
	}
	if !strings.Contains(err.Error(), "sync directory does not exist") {
		t.Fatalf("expected 'sync directory does not exist', got %q", err.Error())
	}
	if !strings.Contains(err.Error(), nonexistent) {
		t.Fatalf("expected error to contain path %q, got %q", nonexistent, err.Error())
	}
}

func TestSyncCommand_ConfigPullDryRunDoesNotUpdateSHA(t *testing.T) {
	t.Parallel()
	cfg := config.NewConfig()
	root := t.TempDir()
	cfg.SetGlobalOption("sync.local-path", root)
	cfg.SetGlobalOption("sync.config-sync", "true")

	originalSHA := "original-test-sha"
	cfg.SetGlobalOption("sync.config-sha", originalSHA)

	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	content := "# osm-shared-config-version 1\ngoal.autodiscovery true\n"
	if err := os.WriteFile(filepath.Join(configDir, "shared.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := NewSyncCommand(cfg)
	var stdout, stderr bytes.Buffer
	err := cmd.Execute([]string{"config-pull", "--dry-run"}, &stdout, &stderr)
	if err != nil {
		t.Fatalf("config-pull --dry-run failed: %v\nstderr: %s", err, stderr.String())
	}

	if !strings.Contains(stdout.String(), "Dry run") {
		t.Fatalf("expected dry-run output, got %q", stdout.String())
	}

	// SHA must remain unchanged.
	if got := cfg.GetString("sync.config-sha"); got != originalSHA {
		t.Fatalf("expected SHA to remain %q after dry-run, got %q", originalSHA, got)
	}

	// Config values must NOT have been applied.
	if v := cfg.GetString("goal.autodiscovery"); v == "true" {
		t.Fatal("expected goal.autodiscovery to NOT be set after dry-run")
	}
}

// --- printConfigDiffSummary unit tests ---

func TestPrintConfigDiffSummary(t *testing.T) {
	t.Parallel()

	t.Run("all_categories", func(t *testing.T) {
		t.Parallel()
		summary := configDiffSummary{
			added: []configKeyValue{
				{key: "new.key", value: "newval"},
			},
			updated: []configKeyValue{
				{key: "changed.key", value: "updatedval"},
			},
			unchanged: []string{"same.key"},
		}
		var buf bytes.Buffer
		printConfigDiffSummary(&buf, summary)
		out := buf.String()

		if !strings.Contains(out, "1 added") {
			t.Errorf("expected '1 added' in output, got %q", out)
		}
		if !strings.Contains(out, "1 updated") {
			t.Errorf("expected '1 updated' in output, got %q", out)
		}
		if !strings.Contains(out, "1 unchanged") {
			t.Errorf("expected '1 unchanged' in output, got %q", out)
		}
		if !strings.Contains(out, "+ new.key = newval") {
			t.Errorf("expected '+ new.key = newval' in output, got %q", out)
		}
		if !strings.Contains(out, "~ changed.key = updatedval") {
			t.Errorf("expected '~ changed.key = updatedval' in output, got %q", out)
		}
		if !strings.Contains(out, "= same.key") {
			t.Errorf("expected '= same.key' in output, got %q", out)
		}
	})

	t.Run("empty_summary", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		printConfigDiffSummary(&buf, configDiffSummary{})
		out := buf.String()
		if !strings.Contains(out, "0 added, 0 updated, 0 unchanged") {
			t.Errorf("expected zero counts in output, got %q", out)
		}
	})

	t.Run("multiple_entries", func(t *testing.T) {
		t.Parallel()
		summary := configDiffSummary{
			added: []configKeyValue{
				{key: "a", value: "1"},
				{key: "b", value: "2"},
			},
			updated: []configKeyValue{
				{key: "c", value: "3"},
			},
			unchanged: []string{"d", "e", "f"},
		}
		var buf bytes.Buffer
		printConfigDiffSummary(&buf, summary)
		out := buf.String()

		if !strings.Contains(out, "2 added, 1 updated, 3 unchanged") {
			t.Errorf("expected '2 added, 1 updated, 3 unchanged', got %q", out)
		}
		// Verify all added entries present.
		if !strings.Contains(out, "+ a = 1") || !strings.Contains(out, "+ b = 2") {
			t.Errorf("missing added entries in output: %q", out)
		}
	})
}
