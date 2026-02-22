package command

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxGoalFileSize is the maximum size of a goal JSON file (1 MiB).
// Files larger than this are rejected to prevent accidental loading of
// non-goal files or denial-of-service from pathological inputs.
const maxGoalFileSize = 1 << 20

// maxDirEntries is the maximum number of directory entries to scan in a
// single goal directory. Directories exceeding this limit are truncated
// with a warning to prevent excessive scanning in pathological cases.
const maxDirEntries = 10000

// LoadGoalFromFile loads a goal definition from a JSON file.
// It validates required fields and resolves script content from embedded or external files.
func LoadGoalFromFile(path string) (*Goal, error) {
	// Check file size before reading to reject obviously oversized files
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat goal file %q: %w", path, err)
	}
	if info.Size() > maxGoalFileSize {
		return nil, fmt.Errorf("goal file %q is too large (%d bytes, max %d)", path, info.Size(), maxGoalFileSize)
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read goal file %q: %w", path, err)
	}

	// Unmarshal JSON
	var goal Goal
	if err := json.Unmarshal(data, &goal); err != nil {
		return nil, fmt.Errorf("failed to parse goal JSON in %q: %w", path, err)
	}

	// Validate required fields
	if err := validateGoal(&goal); err != nil {
		return nil, fmt.Errorf("invalid goal definition in %q: %w", path, err)
	}

	// Set FileName to basename of the definition file
	goal.FileName = filepath.Base(path)

	// Resolve script content
	if err := resolveGoalScript(&goal); err != nil {
		return nil, fmt.Errorf("failed to resolve goal script for %q: %w", path, err)
	}

	return &goal, nil
}

// validateGoal checks that all required fields are present and valid
func validateGoal(goal *Goal) error {
	// Name is required and must be a valid identifier
	if goal.Name == "" {
		return fmt.Errorf("Name is required")
	}
	if !isValidGoalName(goal.Name) {
		return fmt.Errorf("Name must be alphanumeric with hyphens only (no spaces): %q", goal.Name)
	}

	// Description is required
	if goal.Description == "" {
		return fmt.Errorf("Description is required")
	}

	// Category is optional but recommended
	// Usage is optional
	// All other fields are optional and will use defaults

	return nil
}

// isValidGoalName checks if a goal name is a valid identifier
func isValidGoalName(name string) bool {
	// Must contain only alphanumeric characters and hyphens, no spaces
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9-]*$`, name)
	return matched
}

// resolveGoalScript resolves the script content for a goal.
// If goal.Script is already set, it is used directly.
// Otherwise, it uses the default goal interpreter.
func resolveGoalScript(goal *Goal) error {
	if goal.Script != "" {
		return nil
	}
	goal.Script = goalScript
	return nil
}

// GoalFileCandidate represents a potential goal definition file
type GoalFileCandidate struct {
	Path string
	Name string
}

// FindGoalFiles scans a directory for goal definition files (*.json).
// Permission errors on individual entries are skipped with a log warning rather
// than failing the entire scan.
func FindGoalFiles(dir string) ([]GoalFileCandidate, error) {
	var candidates []GoalFileCandidate

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Directory doesn't exist, return empty list
		}
		if os.IsPermission(err) {
			// Permission denied reading the directory — skip with nil error
			// since this is expected for some system directories
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read goal directory %q: %w", dir, err)
	}

	// Protect against extremely large directories
	if len(entries) > maxDirEntries {
		log.Printf("warning: goal directory %q contains %d entries (limit %d), truncating scan", dir, len(entries), maxDirEntries)
		entries = entries[:maxDirEntries]
	}

	var skippedDirs, skippedNonJSON, skippedSymlinks int

	for _, entry := range entries {
		// Resolve entry type for symlinks
		if entry.Type()&os.ModeSymlink != 0 {
			info, err := os.Stat(filepath.Join(dir, entry.Name()))
			if err != nil {
				log.Printf("warning: skipping broken symlink %q in %q: %v", entry.Name(), dir, err)
				skippedSymlinks++
				continue
			}
			if info.IsDir() {
				skippedDirs++
				continue
			}
		} else if entry.IsDir() {
			skippedDirs++
			continue
		}

		name := entry.Name()
		// Look for .json files
		if !strings.HasSuffix(strings.ToLower(name), ".json") {
			skippedNonJSON++
			continue
		}

		path := filepath.Join(dir, name)
		// Extract goal name from filename (remove .json extension)
		goalName := strings.TrimSuffix(name, filepath.Ext(name))

		candidates = append(candidates, GoalFileCandidate{
			Path: path,
			Name: goalName,
		})
	}

	if len(candidates) == 0 && len(entries) > 0 {
		log.Printf("warning: goal directory %q exists but contains no .json goal files (%d entries: %d dirs, %d non-json, %d broken symlinks)",
			dir, len(entries), skippedDirs, skippedNonJSON, skippedSymlinks)
	}

	return candidates, nil
}
