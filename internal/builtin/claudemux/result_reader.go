package claudemux

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ClassificationResult is a map of file paths to category names, as written
// by the MCP reportClassification tool.
type ClassificationResult map[string]string

// SplitPlanStage describes one stage in a PR split plan, as written by the
// MCP reportSplitPlan tool.
type SplitPlanStage struct {
	Name         string   `json:"name"`
	Files        []string `json:"files"`
	Message      string   `json:"message"`
	Order        int      `json:"order"`
	Rationale    string   `json:"rationale,omitempty"`    // Why these files are grouped
	Independent  bool     `json:"independent,omitempty"`  // Can merge independently
	EstConflicts int      `json:"estConflicts,omitempty"` // Estimated merge conflicts (0 = none expected)
}

// SplitPlanResult is an ordered list of split plan stages.
type SplitPlanResult []SplitPlanStage

// ReadClassificationResult reads and parses the classification.json file
// from the given result directory. Returns os.ErrNotExist-wrapping error
// if the file does not exist.
func ReadClassificationResult(dir string) (ClassificationResult, error) {
	data, err := os.ReadFile(filepath.Join(dir, "classification.json"))
	if err != nil {
		return nil, fmt.Errorf("claudemux: read classification result: %w", err)
	}
	var result ClassificationResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("claudemux: parse classification result: %w", err)
	}
	return result, nil
}

// ReadSplitPlanResult reads and parses the split-plan.json file from the
// given result directory. Returns os.ErrNotExist-wrapping error if the
// file does not exist.
func ReadSplitPlanResult(dir string) (SplitPlanResult, error) {
	data, err := os.ReadFile(filepath.Join(dir, "split-plan.json"))
	if err != nil {
		return nil, fmt.Errorf("claudemux: read split plan result: %w", err)
	}
	var result SplitPlanResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("claudemux: parse split plan result: %w", err)
	}
	return result, nil
}
