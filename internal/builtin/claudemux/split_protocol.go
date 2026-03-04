package claudemux

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Split protocol types enable bidirectional communication between osm (the
// orchestrator) and an AI agent (e.g. Claude Code) for automated PR splitting.
//
// The protocol flows:
//
//	osm → ClassificationRequest → agent (classify files)
//	agent → ClassificationResult → osm (file→category mapping)
//	osm → SplitPlanRequest → agent (plan the split)
//	agent → SplitPlanProposal → osm (ordered stages)
//	osm → ConflictReport → agent (verification failure)
//	agent → ConflictResolution → osm (proposed fix)
//	osm → SteeringInstruction → agent (mid-task redirection)
//	agent → InstructionAck → osm (acknowledge receipt)

// ---------------------------------------------------------------------------
//  Classification (osm→agent request, agent→osm result)
// ---------------------------------------------------------------------------

// RepoContext provides repository metadata for classification.
type RepoContext struct {
	ModulePath string `json:"modulePath,omitempty"` // Go module path from go.mod
	Language   string `json:"language,omitempty"`   // Primary language (go, js, python, etc.)
	BaseRef    string `json:"baseRef,omitempty"`    // Base branch or ref
}

// ClassificationRequest is sent by osm to an agent requesting file classification.
type ClassificationRequest struct {
	SessionID string            `json:"sessionId"`
	Files     map[string]string `json:"files"`     // path → git status (A, M, D, R)
	Context   RepoContext       `json:"context"`   // repository metadata
	MaxGroups int               `json:"maxGroups"` // 0 = no limit
}

// ValidateClassificationRequest validates a classification request.
func ValidateClassificationRequest(r *ClassificationRequest) error {
	if r.SessionID == "" {
		return errors.New("sessionId is required")
	}
	if len(r.Files) == 0 {
		return errors.New("files must not be empty")
	}
	for path := range r.Files {
		if err := validateFilePath(path); err != nil {
			return fmt.Errorf("invalid file path %q: %w", path, err)
		}
	}
	if r.MaxGroups < 0 {
		return errors.New("maxGroups must be non-negative")
	}
	return nil
}

// ClassificationResponse is the agent's full response with file→category
// mapping plus optional metadata. This is richer than ClassificationResult
// (which is just map[string]string); it includes confidence, rationale, etc.
type ClassificationResponse struct {
	Files            map[string]string  `json:"files"`                      // path → category
	Confidence       map[string]float64 `json:"confidence,omitempty"`       // path → 0.0-1.0
	GroupNames       []string           `json:"groupNames,omitempty"`       // suggested group names
	IndependentPairs [][]string         `json:"independentPairs,omitempty"` // pairs of groups that can merge independently
	Rationale        map[string]string  `json:"rationale,omitempty"`        // category → explanation
}

// ValidateClassificationResponse validates a classification response.
func ValidateClassificationResponse(r *ClassificationResponse) error {
	if len(r.Files) == 0 {
		return errors.New("files must not be empty")
	}
	for path, cat := range r.Files {
		if err := validateFilePath(path); err != nil {
			return fmt.Errorf("invalid file path %q: %w", path, err)
		}
		if strings.TrimSpace(cat) == "" {
			return fmt.Errorf("empty category for file %q", path)
		}
	}
	for path, conf := range r.Confidence {
		if conf < 0 || conf > 1 {
			return fmt.Errorf("confidence for %q out of range [0,1]: %f", path, conf)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
//  Split Plan (osm→agent request, agent→osm proposal)
// ---------------------------------------------------------------------------

// SplitPlanConstraints are sent by osm to constrain plan generation.
type SplitPlanConstraints struct {
	MaxFilesPerSplit  int    `json:"maxFilesPerSplit,omitempty"`  // 0 = no limit
	BranchPrefix      string `json:"branchPrefix,omitempty"`      // e.g. "split/"
	PreferIndependent bool   `json:"preferIndependent,omitempty"` // prefer non-stacked splits
}

// SplitPlanRequest is sent by osm requesting a split plan.
type SplitPlanRequest struct {
	SessionID      string               `json:"sessionId"`
	Classification map[string]string    `json:"classification"` // file → category
	Constraints    SplitPlanConstraints `json:"constraints"`
}

// ValidateSplitPlanRequest validates a split plan request.
func ValidateSplitPlanRequest(r *SplitPlanRequest) error {
	if r.SessionID == "" {
		return errors.New("sessionId is required")
	}
	if len(r.Classification) == 0 {
		return errors.New("classification must not be empty")
	}
	if r.Constraints.MaxFilesPerSplit < 0 {
		return errors.New("maxFilesPerSplit must be non-negative")
	}
	return nil
}

// SplitPlanProposal is the agent's proposed split plan.
// Uses SplitPlanStage defined in result_reader.go (extended with optional fields).
type SplitPlanProposal struct {
	SessionID string           `json:"sessionId"`
	Stages    []SplitPlanStage `json:"stages"`
}

// ValidateSplitPlanProposal validates a split plan proposal.
func ValidateSplitPlanProposal(p *SplitPlanProposal) error {
	if p.SessionID == "" {
		return errors.New("sessionId is required")
	}
	if len(p.Stages) == 0 {
		return errors.New("stages must not be empty")
	}
	seen := make(map[string]string) // file → stage name (for duplicate detection)
	for i, s := range p.Stages {
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("stage %d: name is required", i)
		}
		if len(s.Files) == 0 {
			return fmt.Errorf("stage %q: files must not be empty", s.Name)
		}
		for _, f := range s.Files {
			if err := validateFilePath(f); err != nil {
				return fmt.Errorf("stage %q: invalid file path %q: %w", s.Name, f, err)
			}
			if prev, ok := seen[f]; ok {
				return fmt.Errorf("duplicate file %q in stages %q and %q", f, prev, s.Name)
			}
			seen[f] = s.Name
		}
		if s.EstConflicts < 0 {
			return fmt.Errorf("stage %q: estConflicts must be non-negative", s.Name)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
//  Conflict Resolution (osm→agent report, agent→osm resolution)
// ---------------------------------------------------------------------------

// ConflictReport is sent by osm when a split branch fails verification.
type ConflictReport struct {
	SessionID    string   `json:"sessionId"`
	BranchName   string   `json:"branchName"`
	VerifyOutput string   `json:"verifyOutput"`           // stdout + stderr from verify command
	ExitCode     int      `json:"exitCode"`               // verify command exit code
	Files        []string `json:"files"`                  // files in the failing branch
	GoModContent string   `json:"goModContent,omitempty"` // go.mod content if applicable
}

// ValidateConflictReport validates a conflict report.
func ValidateConflictReport(r *ConflictReport) error {
	if r.SessionID == "" {
		return errors.New("sessionId is required")
	}
	if strings.TrimSpace(r.BranchName) == "" {
		return errors.New("branchName is required")
	}
	if len(r.Files) == 0 {
		return errors.New("files must not be empty")
	}
	return nil
}

// FilePatch describes a file content replacement proposed by the agent.
type FilePatch struct {
	File    string `json:"file"`    // File path to patch
	Content string `json:"content"` // New file content (full replacement)
}

// ConflictResolution is the agent's proposed fix for a verification failure.
type ConflictResolution struct {
	SessionID        string      `json:"sessionId"`
	BranchName       string      `json:"branchName"`
	Patches          []FilePatch `json:"patches,omitempty"`          // File content replacements
	Commands         []string    `json:"commands,omitempty"`         // Commands to run (e.g. "go mod tidy")
	ReSplitSuggested bool        `json:"reSplitSuggested,omitempty"` // Suggest re-classification
	ReSplitReason    string      `json:"reSplitReason,omitempty"`    // Why re-split is needed
}

// ValidateConflictResolution validates a conflict resolution.
func ValidateConflictResolution(r *ConflictResolution) error {
	if r.SessionID == "" {
		return errors.New("sessionId is required")
	}
	if strings.TrimSpace(r.BranchName) == "" {
		return errors.New("branchName is required")
	}
	if len(r.Patches) == 0 && len(r.Commands) == 0 && !r.ReSplitSuggested {
		return errors.New("resolution must include patches, commands, or re-split suggestion")
	}
	for i, p := range r.Patches {
		if err := validateFilePath(p.File); err != nil {
			return fmt.Errorf("patch %d: invalid file path %q: %w", i, p.File, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
//  Bidirectional Steering (osm→agent instruction, agent→osm ack)
// ---------------------------------------------------------------------------

// SteeringType identifies the kind of steering instruction.
type SteeringType string

const (
	SteeringAbort      SteeringType = "abort"
	SteeringModifyPlan SteeringType = "modify-plan"
	SteeringReClassify SteeringType = "re-classify"
	SteeringFocus      SteeringType = "focus"
)

// SteeringInstruction is sent by osm to redirect the agent mid-task.
type SteeringInstruction struct {
	SessionID string       `json:"sessionId"`
	Type      SteeringType `json:"type"`
	Payload   any          `json:"payload,omitempty"` // type-dependent payload
}

// ValidateSteeringInstruction validates a steering instruction.
func ValidateSteeringInstruction(i *SteeringInstruction) error {
	if i.SessionID == "" {
		return errors.New("sessionId is required")
	}
	switch i.Type {
	case SteeringAbort, SteeringModifyPlan, SteeringReClassify, SteeringFocus:
		// ok
	default:
		return fmt.Errorf("unknown steering type: %q", i.Type)
	}
	return nil
}

// AckStatus indicates the agent's response to a steering instruction.
type AckStatus string

const (
	AckReceived  AckStatus = "received"
	AckExecuting AckStatus = "executing"
	AckCompleted AckStatus = "completed"
	AckRejected  AckStatus = "rejected"
)

// InstructionAck is the agent's acknowledgement of a steering instruction.
type InstructionAck struct {
	SessionID       string       `json:"sessionId"`
	InstructionType SteeringType `json:"instructionType"`
	Status          AckStatus    `json:"status"`
	Message         string       `json:"message,omitempty"`
}

// ValidateInstructionAck validates an instruction acknowledgement.
func ValidateInstructionAck(a *InstructionAck) error {
	if a.SessionID == "" {
		return errors.New("sessionId is required")
	}
	switch a.InstructionType {
	case SteeringAbort, SteeringModifyPlan, SteeringReClassify, SteeringFocus:
		// ok
	default:
		return fmt.Errorf("unknown instruction type: %q", a.InstructionType)
	}
	switch a.Status {
	case AckReceived, AckExecuting, AckCompleted, AckRejected:
		// ok
	default:
		return fmt.Errorf("unknown ack status: %q", a.Status)
	}
	return nil
}

// ---------------------------------------------------------------------------
//  Shared validation helpers
// ---------------------------------------------------------------------------

func validateFilePath(path string) error {
	if path == "" {
		return errors.New("empty path")
	}
	if filepath.IsAbs(path) {
		return errors.New("absolute paths not allowed")
	}
	if strings.Contains(path, "..") {
		return errors.New("path traversal not allowed")
	}
	return nil
}
