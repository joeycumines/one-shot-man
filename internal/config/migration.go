package config

import (
	"fmt"
	"maps"
	"strconv"
)

// SchemaVersion is the current configuration schema version.
const SchemaVersion = 1

// MigrationResult describes what changed during a config migration.
type MigrationResult struct {
	FromVersion int
	ToVersion   int
	Changes     []string
}

// CheckSchemaVersion returns validation issues related to the config's
// schema version. It does NOT modify the config. Returns an empty slice
// if the version is current. Internally uses MigrateConfig on a shallow
// copy to determine the version delta.
func CheckSchemaVersion(c *Config) []string {
	// Work on a shallow copy to avoid mutating the original.
	probe := &Config{
		Global: make(map[string]string, len(c.Global)),
	}
	maps.Copy(probe.Global, c.Global)

	result, err := MigrateConfig(probe)
	if err != nil {
		return []string{err.Error()}
	}
	if result == nil {
		return nil
	}
	return []string{fmt.Sprintf("schema version %d is outdated, current is %d (run migration to update)", result.FromVersion, result.ToVersion)}
}

// MigrateConfig migrates a Config to the current schema version.
// A Config with no config.schema-version key is treated as version 0.
// Returns (nil, nil) if the config is already at the current version.
// Returns an error if the config version is newer than supported.
func MigrateConfig(c *Config) (*MigrationResult, error) {
	from := detectSchemaVersion(c)

	if from == SchemaVersion {
		return nil, nil
	}

	if from > SchemaVersion {
		return nil, fmt.Errorf("config schema version %d is newer than supported version %d", from, SchemaVersion)
	}

	result := &MigrationResult{
		FromVersion: from,
		ToVersion:   SchemaVersion,
	}

	// Apply migrations sequentially.
	for v := from; v < SchemaVersion; v++ {
		fn, ok := migrations[v]
		if !ok {
			return nil, fmt.Errorf("no migration registered for version %d → %d", v, v+1)
		}
		changes, err := fn(c)
		if err != nil {
			return nil, fmt.Errorf("migration %d → %d failed: %w", v, v+1, err)
		}
		result.Changes = append(result.Changes, changes...)
	}

	// Stamp the new version.
	c.SetGlobalOption("config.schema-version", strconv.Itoa(SchemaVersion))
	result.Changes = append(result.Changes, fmt.Sprintf("set config.schema-version to %d", SchemaVersion))

	return result, nil
}

// detectSchemaVersion returns the config's schema version.
// Returns 0 if the key is absent or cannot be parsed as an integer.
func detectSchemaVersion(c *Config) int {
	v, ok := c.GetGlobalOption("config.schema-version")
	if !ok {
		return 0
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return i
}

// migrations maps a source version to a function that migrates to the next
// version. Each function returns a list of human-readable change descriptions.
var migrations = map[int]func(c *Config) ([]string, error){
	// v0 → v1: no-op migration — establishes versioning.
	0: migrateV0ToV1,
}

// migrateV0ToV1 is a structural identity migration that establishes
// the versioning system. No actual config keys are altered.
func migrateV0ToV1(c *Config) ([]string, error) {
	_ = c // no-op: v0 and v1 are structurally identical
	return []string{"established schema versioning (v0 → v1)"}, nil
}
