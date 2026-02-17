package command

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/gitops"
)

// SyncAutoPull runs a non-blocking git pull --rebase on the sync repository
// if sync.auto-pull is enabled in the configuration. Errors are written to
// stderr but do not prevent startup.
func SyncAutoPull(cfg *config.Config, stderr io.Writer) {
	if cfg == nil {
		return
	}
	val, exists := cfg.GetGlobalOption("sync.auto-pull")
	if !exists || val != "true" {
		return
	}

	root := syncRootFromConfig(cfg)
	if root == "" {
		return
	}

	if !gitops.IsRepo(root) {
		return
	}

	gitBin := "git"
	cmd := exec.Command(gitBin, "pull", "--rebase", "origin", "HEAD")
	cmd.Dir = root
	cmd.Stdout = io.Discard
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		_, _ = fmt.Fprintf(stderr, "sync auto-pull failed: %v\n", err)
	}
}

// ApplySyncDiscoveryPaths adds the sync repository's goals/ and scripts/
// directories to the goal.paths and script.paths configuration keys if
// those directories exist. This enables automatic discovery of goals and
// scripts from the sync repository.
func ApplySyncDiscoveryPaths(cfg *config.Config) {
	if cfg == nil {
		return
	}

	root := syncRootFromConfig(cfg)
	if root == "" {
		return
	}

	// Only inject paths if the sync root actually exists.
	if _, err := os.Stat(root); err != nil {
		return
	}

	goalDir := filepath.Join(root, "goals")
	if info, err := os.Stat(goalDir); err == nil && info.IsDir() {
		appendConfigPath(cfg, "goal.paths", goalDir)
	}

	scriptDir := filepath.Join(root, "scripts")
	if info, err := os.Stat(scriptDir); err == nil && info.IsDir() {
		appendConfigPath(cfg, "script.paths", scriptDir)
		// Also add to module-paths for require() resolution.
		appendConfigPath(cfg, "script.module-paths", scriptDir)
	}
}

// syncRootFromConfig resolves the sync root directory from configuration.
// Returns empty string if no path can be determined.
func syncRootFromConfig(cfg *config.Config) string {
	if p, exists := cfg.GetGlobalOption("sync.local-path"); exists && p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".osm", "sync")
}

// appendConfigPath appends a path to a colon-separated config key value.
func appendConfigPath(cfg *config.Config, key, path string) {
	existing, _ := cfg.GetGlobalOption(key)
	if existing == "" {
		cfg.SetGlobalOption(key, path)
	} else {
		// Check if path is already present.
		for _, p := range filepath.SplitList(existing) {
			if p == path {
				return
			}
		}
		cfg.SetGlobalOption(key, existing+string(os.PathListSeparator)+path)
	}
}
