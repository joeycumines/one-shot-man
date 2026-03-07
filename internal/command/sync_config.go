package command

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// sharedConfigVersion is the current schema version for shared config files.
// Bumping this value makes older osm versions refuse to import the file.
const sharedConfigVersion = 1

// sharedConfigHeader is the magic first line of a shared config file.
const sharedConfigHeader = "# osm-shared-config-version"

// sharedConfigRelPath is the path within the sync root to the shared config.
const sharedConfigRelPath = "config/shared.conf"

// sensitiveKeyPrefixes lists key prefixes that are NEVER synced. These
// contain machine-local or security-sensitive values.
var sensitiveKeyPrefixes = []string{
	"sync.",
	"log.file",
	"session.",
}

// isSensitiveKey reports whether key must be excluded from shared config.
func isSensitiveKey(key string) bool {
	for _, prefix := range sensitiveKeyPrefixes {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

// executeConfigPush writes shareable global config keys to the sync repo's
// config/shared.conf. Sensitive keys are excluded. The SHA256 of the written
// content is stored in sync.config-sha for conflict detection on pull.
func (c *SyncCommand) executeConfigPush(args []string, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		return fmt.Errorf("%w for config-push: %v", ErrUnexpectedArguments, args)
	}

	if c.config == nil {
		return fmt.Errorf("no configuration loaded")
	}

	if !c.config.GetBool("sync.config-sync") {
		_, _ = fmt.Fprintln(stderr, "Config sync is disabled. Set sync.config-sync=true to enable.")
		return &SilentError{Err: fmt.Errorf("config sync disabled: set sync.config-sync=true")}
	}

	root, err := c.syncRoot()
	if err != nil {
		return err
	}

	// Acquire sync lock.
	unlock, err := syncConfigLock(root)
	if err != nil {
		return err
	}
	defer unlock()

	// Collect shareable keys.
	keys := make([]string, 0, len(c.config.Global))
	for k := range c.config.Global {
		if !isSensitiveKey(k) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)

	if len(keys) == 0 {
		_, _ = fmt.Fprintln(stdout, "No shareable config keys to push.")
		return nil
	}

	// Build shared.conf content.
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s %d\n", sharedConfigHeader, sharedConfigVersion))
	for _, k := range keys {
		sb.WriteString(fmt.Sprintf("%s %s\n", k, c.config.Global[k]))
	}
	content := sb.String()

	// Ensure config directory exists.
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	// Atomic write: write to temp file, then rename.
	sharedPath := filepath.Join(root, sharedConfigRelPath)
	tmpPath := sharedPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing shared config temp file: %w", err)
	}
	if err := os.Rename(tmpPath, sharedPath); err != nil {
		// Clean up temp file on rename failure.
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming shared config temp file: %w", err)
	}

	// Check if shared.conf is gitignored.
	if checkGitignored(root, sharedPath) {
		_, _ = fmt.Fprintln(stderr, "Warning: shared.conf is gitignored and won't be committed")
	}

	// Store SHA for conflict detection.
	sha := sha256Hex(content)
	c.config.SetGlobalOption("sync.config-sha", sha)

	_, _ = fmt.Fprintf(stdout, "Pushed %d config keys to shared config.\n", len(keys))
	_, _ = fmt.Fprintf(stdout, "SHA: %s\n", sha)
	_, _ = fmt.Fprintln(stdout, "Run 'osm sync push' to commit and send to remote.")
	return nil
}

// executeConfigPull reads the sync repo's config/shared.conf and merges
// keys into the running configuration. Conflict handling:
//   - No stored SHA (first pull)     → require --force
//   - Stored SHA matches remote file → already applied, no-op
//   - Stored SHA differs             → remote changed, auto-apply
func (c *SyncCommand) executeConfigPull(args []string, stdout, stderr io.Writer) error {
	var force bool
	var dryRun bool
	for _, a := range args {
		switch a {
		case "--force", "-f":
			force = true
		case "--dry-run":
			dryRun = true
		default:
			return fmt.Errorf("%w for config-pull: %s", ErrUnexpectedArguments, a)
		}
	}

	if c.config == nil {
		return fmt.Errorf("no configuration loaded")
	}

	if !c.config.GetBool("sync.config-sync") {
		_, _ = fmt.Fprintln(stderr, "Config sync is disabled. Set sync.config-sync=true to enable.")
		return &SilentError{Err: fmt.Errorf("config sync disabled: set sync.config-sync=true")}
	}

	root, err := c.syncRoot()
	if err != nil {
		return err
	}

	// Check that the sync root directory exists (pull requires it).
	if _, statErr := os.Stat(root); errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("sync directory does not exist: %s", root)
	}

	// Acquire sync lock (unless dry-run, which is read-only).
	if !dryRun {
		unlock, lockErr := syncConfigLock(root)
		if lockErr != nil {
			return lockErr
		}
		defer unlock()
	}

	sharedPath := filepath.Join(root, sharedConfigRelPath)
	data, err := os.ReadFile(sharedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_, _ = fmt.Fprintln(stderr, "No shared config found in sync repository.")
			_, _ = fmt.Fprintln(stderr, "Use 'osm sync config-push' to publish your config first.")
			return &SilentError{Err: fmt.Errorf("shared config not found: %s", sharedConfigRelPath)}
		}
		return fmt.Errorf("reading shared config: %w", err)
	}
	content := string(data)

	// Parse and validate version.
	version, configKeys, err := parseSharedConfig(content)
	if err != nil {
		return fmt.Errorf("parsing shared config: %w", err)
	}
	if version > sharedConfigVersion {
		_, _ = fmt.Fprintf(stderr, "Shared config schema version %d is newer than supported (%d).\n",
			version, sharedConfigVersion)
		_, _ = fmt.Fprintln(stderr, "Upgrade osm to apply this configuration.")
		return &SilentError{Err: fmt.Errorf("unsupported shared config version %d (max %d)", version, sharedConfigVersion)}
	}

	// Compute remote SHA.
	remoteSHA := sha256Hex(content)

	// Check for conflict.
	storedSHA := c.config.GetString("sync.config-sha")
	if storedSHA == "" && !force {
		_, _ = fmt.Fprintln(stderr, "No previous sync state found (first pull).")
		_, _ = fmt.Fprintln(stderr, "Local config may have been manually configured.")
		_, _ = fmt.Fprintln(stderr, "Use --force to apply shared config anyway.")
		return &SilentError{Err: fmt.Errorf("unknown sync state: use --force for first pull")}
	}

	if storedSHA == remoteSHA && !dryRun {
		_, _ = fmt.Fprintln(stdout, "Shared config already up to date (SHA matches).")
		return nil
	}

	// Compute and print conflict summary.
	summary := computeConfigDiff(c.config.Global, configKeys)
	printConfigDiffSummary(stdout, summary)

	// In dry-run mode, stop here — do not apply or update SHA.
	if dryRun {
		_, _ = fmt.Fprintln(stdout, "Dry run: no changes applied.")
		return nil
	}

	// Apply remote keys. Skip sensitive keys that somehow got into the file.
	applied := 0
	for _, kv := range configKeys {
		if isSensitiveKey(kv.key) {
			_, _ = fmt.Fprintf(stderr, "Warning: skipping sensitive key in shared config: %s\n", kv.key)
			continue
		}
		c.config.SetGlobalOption(kv.key, kv.value)
		applied++
	}

	// Update stored SHA.
	c.config.SetGlobalOption("sync.config-sha", remoteSHA)

	_, _ = fmt.Fprintf(stdout, "Applied %d config keys from shared config.\n", applied)
	_, _ = fmt.Fprintf(stdout, "SHA: %s\n", remoteSHA)
	_, _ = fmt.Fprintln(stdout, "Note: changes are in-memory only. Update your config file to persist.")
	return nil
}

// configKeyValue is a parsed key-value pair from a shared config file.
type configKeyValue struct {
	key   string
	value string
}

// parseSharedConfig parses a shared.conf file, returning the schema version
// and the key-value pairs. Returns an error if the version header is missing
// or malformed.
func parseSharedConfig(content string) (int, []configKeyValue, error) {
	scanner := bufio.NewScanner(strings.NewReader(content))

	// First line must be the version header.
	if !scanner.Scan() {
		return 0, nil, fmt.Errorf("empty shared config file")
	}
	firstLine := scanner.Text()
	version, err := parseVersionHeader(firstLine)
	if err != nil {
		return 0, nil, err
	}

	var keys []configKeyValue
	lineNum := 1
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// Skip empty lines and comments.
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse "key value" format (dnsmasq-style: first token is key,
		// remainder is value).
		idx := strings.IndexByte(line, ' ')
		if idx < 0 {
			// Key with no value — treat as key with empty value.
			keys = append(keys, configKeyValue{key: line, value: ""})
			continue
		}

		key := line[:idx]
		value := line[idx+1:]
		keys = append(keys, configKeyValue{key: key, value: value})
	}

	if err := scanner.Err(); err != nil {
		return 0, nil, fmt.Errorf("reading shared config: %w", err)
	}

	return version, keys, nil
}

// parseVersionHeader parses the "# osm-shared-config-version N" header line.
func parseVersionHeader(line string) (int, error) {
	rest, ok := strings.CutPrefix(line, sharedConfigHeader)
	if !ok {
		return 0, fmt.Errorf("missing version header (expected %q prefix)", sharedConfigHeader)
	}
	versionStr := strings.TrimSpace(rest)
	if versionStr == "" {
		return 0, fmt.Errorf("version header missing version number")
	}
	var version int
	if _, err := fmt.Sscanf(versionStr, "%d", &version); err != nil {
		return 0, fmt.Errorf("invalid version number %q: %w", versionStr, err)
	}
	if version < 1 {
		return 0, fmt.Errorf("invalid version number %d (must be >= 1)", version)
	}
	return version, nil
}

// sha256Hex computes the hex-encoded SHA256 of a string.
func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

// configDiffSummary describes the differences between local and remote config.
type configDiffSummary struct {
	added     []configKeyValue // keys in remote but not in local
	updated   []configKeyValue // keys in both but different values
	unchanged []string         // keys in both with same values
}

// computeConfigDiff compares local config values against remote key-value
// pairs. Only non-sensitive remote keys are considered.
func computeConfigDiff(localConfig map[string]string, remoteKeys []configKeyValue) configDiffSummary {
	var s configDiffSummary
	for _, kv := range remoteKeys {
		if isSensitiveKey(kv.key) {
			continue
		}
		localVal, exists := localConfig[kv.key]
		if !exists {
			s.added = append(s.added, kv)
		} else if localVal != kv.value {
			s.updated = append(s.updated, kv)
		} else {
			s.unchanged = append(s.unchanged, kv.key)
		}
	}
	return s
}

// printConfigDiffSummary writes a human-readable diff summary to w.
func printConfigDiffSummary(w io.Writer, summary configDiffSummary) {
	_, _ = fmt.Fprintf(w, "Config diff: %d added, %d updated, %d unchanged\n",
		len(summary.added), len(summary.updated), len(summary.unchanged))
	for _, kv := range summary.added {
		_, _ = fmt.Fprintf(w, "  + %s = %s\n", kv.key, kv.value)
	}
	for _, kv := range summary.updated {
		_, _ = fmt.Fprintf(w, "  ~ %s = %s\n", kv.key, kv.value)
	}
	for _, k := range summary.unchanged {
		_, _ = fmt.Fprintf(w, "  = %s\n", k)
	}
}

// syncConfigLockPath returns the path of the sync-config lock file.
func syncConfigLockPath(root string) string {
	return filepath.Join(root, "config", ".sync-config.lock")
}

// syncConfigLockMaxAge is the maximum age of a lock file before it is
// considered stale and automatically removed.
const syncConfigLockMaxAge = 10 * time.Minute

// syncConfigLock acquires an advisory lock file in the sync config directory.
// Returns a cleanup function that releases the lock. If a stale lock is
// detected (the owning PID is no longer running, or the lock is older than
// syncConfigLockMaxAge), it is automatically removed and re-acquired.
func syncConfigLock(root string) (func(), error) {
	lockPath := syncConfigLockPath(root)

	// Ensure the parent directory exists.
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, fmt.Errorf("creating lock directory: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			if removeStaleLock(lockPath) {
				// Stale lock removed — retry once.
				f, err = os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
				if err != nil {
					return nil, fmt.Errorf("acquiring sync lock after stale removal: %w", err)
				}
			} else {
				return nil, fmt.Errorf("another sync operation is in progress (lock: %s)", lockPath)
			}
		} else {
			return nil, fmt.Errorf("acquiring sync lock: %w", err)
		}
	}

	// Write debugging info.
	_, _ = fmt.Fprintf(f, "pid=%d\ntime=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339))
	_ = f.Close()

	cleanup := func() {
		_ = os.Remove(lockPath)
	}
	return cleanup, nil
}

// removeStaleLock checks whether the lock file at lockPath is stale (owner
// process is dead or the lock is older than syncConfigLockMaxAge). If stale,
// it removes the lock file and returns true. Otherwise returns false.
func removeStaleLock(lockPath string) bool {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}

	lines := strings.Split(string(data), "\n")
	var pid int
	var lockTime time.Time
	for _, line := range lines {
		if after, ok := strings.CutPrefix(line, "pid="); ok {
			pid, _ = strconv.Atoi(after)
		}
		if after, ok := strings.CutPrefix(line, "time="); ok {
			lockTime, _ = time.Parse(time.RFC3339, after)
		}
	}

	// Check if lock is older than max age.
	if !lockTime.IsZero() && time.Since(lockTime) > syncConfigLockMaxAge {
		return os.Remove(lockPath) == nil
	}

	// Check if the owning process is still alive.
	if pid > 0 && !processAlive(pid) {
		return os.Remove(lockPath) == nil
	}

	return false
}

// checkGitignored returns true if the given path is ignored by git in the
// given root directory. Returns false if git is not available or on any error.
func checkGitignored(root, path string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", path)
	cmd.Dir = root
	err := cmd.Run()
	return err == nil // exit 0 means the file IS gitignored
}
