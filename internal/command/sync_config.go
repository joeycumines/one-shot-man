package command

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
		return fmt.Errorf("unexpected arguments for config-push: %v", args)
	}

	if c.config == nil {
		return fmt.Errorf("no configuration loaded")
	}

	if !c.config.GetBool("sync.config-sync") {
		_, _ = fmt.Fprintln(stderr, "Config sync is disabled. Set sync.config-sync=true to enable.")
		return fmt.Errorf("config sync disabled: set sync.config-sync=true")
	}

	root, err := c.syncRoot()
	if err != nil {
		return err
	}

	// Collect shareable keys.
	keys := make([]string, 0, len(c.config.Global))
	for k := range c.config.Global {
		if !isSensitiveKey(k) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

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

	sharedPath := filepath.Join(root, sharedConfigRelPath)
	if err := os.WriteFile(sharedPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing shared config: %w", err)
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
	for _, a := range args {
		if a == "--force" || a == "-f" {
			force = true
		} else {
			return fmt.Errorf("unexpected argument for config-pull: %s", a)
		}
	}

	if c.config == nil {
		return fmt.Errorf("no configuration loaded")
	}

	if !c.config.GetBool("sync.config-sync") {
		_, _ = fmt.Fprintln(stderr, "Config sync is disabled. Set sync.config-sync=true to enable.")
		return fmt.Errorf("config sync disabled: set sync.config-sync=true")
	}

	root, err := c.syncRoot()
	if err != nil {
		return err
	}

	sharedPath := filepath.Join(root, sharedConfigRelPath)
	data, err := os.ReadFile(sharedPath)
	if err != nil {
		if os.IsNotExist(err) {
			_, _ = fmt.Fprintln(stderr, "No shared config found in sync repository.")
			_, _ = fmt.Fprintln(stderr, "Use 'osm sync config-push' to publish your config first.")
			return fmt.Errorf("shared config not found: %s", sharedConfigRelPath)
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
		return fmt.Errorf("unsupported shared config version %d (max %d)", version, sharedConfigVersion)
	}

	// Compute remote SHA.
	remoteSHA := sha256Hex(content)

	// Check for conflict.
	storedSHA := c.config.GetString("sync.config-sha")
	if storedSHA == "" && !force {
		_, _ = fmt.Fprintln(stderr, "No previous sync state found (first pull).")
		_, _ = fmt.Fprintln(stderr, "Local config may have been manually configured.")
		_, _ = fmt.Fprintln(stderr, "Use --force to apply shared config anyway.")
		return fmt.Errorf("unknown sync state: use --force for first pull")
	}

	if storedSHA == remoteSHA {
		_, _ = fmt.Fprintln(stdout, "Shared config already up to date (SHA matches).")
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
	if !strings.HasPrefix(line, sharedConfigHeader) {
		return 0, fmt.Errorf("missing version header (expected %q prefix)", sharedConfigHeader)
	}
	versionStr := strings.TrimSpace(strings.TrimPrefix(line, sharedConfigHeader))
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
