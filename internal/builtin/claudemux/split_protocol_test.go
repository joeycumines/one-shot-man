package claudemux

import (
	"encoding/json"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
//  ClassificationRequest
// ---------------------------------------------------------------------------

func TestValidateClassificationRequest_Valid(t *testing.T) {
	r := &ClassificationRequest{
		SessionID: "sess-1",
		Files:     map[string]string{"cmd/main.go": "M", "pkg/util.go": "A"},
		Context:   RepoContext{ModulePath: "example.com/foo", Language: "go"},
		MaxGroups: 3,
	}
	if err := ValidateClassificationRequest(r); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateClassificationRequest_MissingSessionID(t *testing.T) {
	r := &ClassificationRequest{
		Files: map[string]string{"a.go": "M"},
	}
	err := ValidateClassificationRequest(r)
	if err == nil || !strings.Contains(err.Error(), "sessionId") {
		t.Errorf("expected sessionId error, got: %v", err)
	}
}

func TestValidateClassificationRequest_EmptyFiles(t *testing.T) {
	r := &ClassificationRequest{
		SessionID: "sess-1",
		Files:     map[string]string{},
	}
	err := ValidateClassificationRequest(r)
	if err == nil || !strings.Contains(err.Error(), "files") {
		t.Errorf("expected files error, got: %v", err)
	}
}

func TestValidateClassificationRequest_InvalidPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"absolute", "/etc/passwd", "absolute"},
		{"traversal", "../etc/passwd", "traversal"},
		{"empty", "", "empty path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ClassificationRequest{
				SessionID: "sess-1",
				Files:     map[string]string{tt.path: "M"},
			}
			err := ValidateClassificationRequest(r)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected %q error, got: %v", tt.want, err)
			}
		})
	}
}

func TestValidateClassificationRequest_NegativeMaxGroups(t *testing.T) {
	r := &ClassificationRequest{
		SessionID: "sess-1",
		Files:     map[string]string{"a.go": "M"},
		MaxGroups: -1,
	}
	err := ValidateClassificationRequest(r)
	if err == nil || !strings.Contains(err.Error(), "maxGroups") {
		t.Errorf("expected maxGroups error, got: %v", err)
	}
}

func TestClassificationRequest_JSONRoundTrip(t *testing.T) {
	orig := ClassificationRequest{
		SessionID: "sess-1",
		Files:     map[string]string{"cmd/main.go": "M", "pkg/util.go": "A"},
		Context:   RepoContext{ModulePath: "example.com/foo", Language: "go", BaseRef: "main"},
		MaxGroups: 5,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got ClassificationRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.SessionID != orig.SessionID {
		t.Errorf("SessionID mismatch: %q vs %q", got.SessionID, orig.SessionID)
	}
	if len(got.Files) != len(orig.Files) {
		t.Errorf("Files count mismatch: %d vs %d", len(got.Files), len(orig.Files))
	}
	if got.Context.ModulePath != orig.Context.ModulePath {
		t.Errorf("Context.ModulePath mismatch: %q vs %q", got.Context.ModulePath, orig.Context.ModulePath)
	}
	if got.MaxGroups != orig.MaxGroups {
		t.Errorf("MaxGroups mismatch: %d vs %d", got.MaxGroups, orig.MaxGroups)
	}
}

// ---------------------------------------------------------------------------
//  ClassificationResponse
// ---------------------------------------------------------------------------

func TestValidateClassificationResponse_Valid(t *testing.T) {
	r := &ClassificationResponse{
		Files:      map[string]string{"cmd/main.go": "impl", "docs/readme.md": "docs"},
		Confidence: map[string]float64{"cmd/main.go": 0.95, "docs/readme.md": 0.8},
		GroupNames: []string{"impl", "docs"},
		Rationale:  map[string]string{"impl": "implementation files", "docs": "documentation"},
	}
	if err := ValidateClassificationResponse(r); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateClassificationResponse_EmptyFiles(t *testing.T) {
	r := &ClassificationResponse{
		Files: map[string]string{},
	}
	err := ValidateClassificationResponse(r)
	if err == nil || !strings.Contains(err.Error(), "files") {
		t.Errorf("expected files error, got: %v", err)
	}
}

func TestValidateClassificationResponse_EmptyCategory(t *testing.T) {
	r := &ClassificationResponse{
		Files: map[string]string{"a.go": ""},
	}
	err := ValidateClassificationResponse(r)
	if err == nil || !strings.Contains(err.Error(), "empty category") {
		t.Errorf("expected empty category error, got: %v", err)
	}
}

func TestValidateClassificationResponse_BadConfidence(t *testing.T) {
	tests := []struct {
		name string
		conf float64
	}{
		{"negative", -0.1},
		{"over one", 1.5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ClassificationResponse{
				Files:      map[string]string{"a.go": "impl"},
				Confidence: map[string]float64{"a.go": tt.conf},
			}
			err := ValidateClassificationResponse(r)
			if err == nil || !strings.Contains(err.Error(), "out of range") {
				t.Errorf("expected out of range error, got: %v", err)
			}
		})
	}
}

func TestClassificationResponse_JSONRoundTrip(t *testing.T) {
	orig := ClassificationResponse{
		Files:            map[string]string{"a.go": "impl", "b.go": "test"},
		Confidence:       map[string]float64{"a.go": 0.9},
		GroupNames:       []string{"impl", "test"},
		IndependentPairs: [][]string{{"impl", "test"}},
		Rationale:        map[string]string{"impl": "core code"},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got ClassificationResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != len(orig.Files) {
		t.Errorf("Files count mismatch: %d vs %d", len(got.Files), len(orig.Files))
	}
	if len(got.IndependentPairs) != 1 || len(got.IndependentPairs[0]) != 2 {
		t.Errorf("IndependentPairs mismatch: %v", got.IndependentPairs)
	}
}

// ---------------------------------------------------------------------------
//  SplitPlanRequest + SplitPlanProposal
// ---------------------------------------------------------------------------

func TestValidateSplitPlanRequest_Valid(t *testing.T) {
	r := &SplitPlanRequest{
		SessionID:      "sess-1",
		Classification: map[string]string{"a.go": "impl", "b.go": "test"},
		Constraints:    SplitPlanConstraints{MaxFilesPerSplit: 10, BranchPrefix: "split/"},
	}
	if err := ValidateSplitPlanRequest(r); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateSplitPlanRequest_EmptyClassification(t *testing.T) {
	r := &SplitPlanRequest{
		SessionID:      "sess-1",
		Classification: map[string]string{},
	}
	err := ValidateSplitPlanRequest(r)
	if err == nil || !strings.Contains(err.Error(), "classification") {
		t.Errorf("expected classification error, got: %v", err)
	}
}

func TestValidateSplitPlanRequest_NegativeMaxFiles(t *testing.T) {
	r := &SplitPlanRequest{
		SessionID:      "sess-1",
		Classification: map[string]string{"a.go": "impl"},
		Constraints:    SplitPlanConstraints{MaxFilesPerSplit: -1},
	}
	err := ValidateSplitPlanRequest(r)
	if err == nil || !strings.Contains(err.Error(), "maxFilesPerSplit") {
		t.Errorf("expected maxFilesPerSplit error, got: %v", err)
	}
}

func TestValidateSplitPlanProposal_Valid(t *testing.T) {
	p := &SplitPlanProposal{
		SessionID: "sess-1",
		Stages: []SplitPlanStage{
			{Name: "types", Files: []string{"types.go"}, Order: 0, Rationale: "type defs"},
			{Name: "impl", Files: []string{"impl.go"}, Order: 1, Independent: true},
		},
	}
	if err := ValidateSplitPlanProposal(p); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateSplitPlanProposal_EmptyStages(t *testing.T) {
	p := &SplitPlanProposal{
		SessionID: "sess-1",
		Stages:    []SplitPlanStage{},
	}
	err := ValidateSplitPlanProposal(p)
	if err == nil || !strings.Contains(err.Error(), "stages") {
		t.Errorf("expected stages error, got: %v", err)
	}
}

func TestValidateSplitPlanProposal_DuplicateFiles(t *testing.T) {
	p := &SplitPlanProposal{
		SessionID: "sess-1",
		Stages: []SplitPlanStage{
			{Name: "a", Files: []string{"shared.go"}, Order: 0},
			{Name: "b", Files: []string{"shared.go"}, Order: 1},
		},
	}
	err := ValidateSplitPlanProposal(p)
	if err == nil || !strings.Contains(err.Error(), "duplicate file") {
		t.Errorf("expected duplicate file error, got: %v", err)
	}
}

func TestValidateSplitPlanProposal_EmptyName(t *testing.T) {
	p := &SplitPlanProposal{
		SessionID: "sess-1",
		Stages:    []SplitPlanStage{{Name: "", Files: []string{"a.go"}}},
	}
	err := ValidateSplitPlanProposal(p)
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected name error, got: %v", err)
	}
}

func TestValidateSplitPlanProposal_EmptyFilesInStage(t *testing.T) {
	p := &SplitPlanProposal{
		SessionID: "sess-1",
		Stages:    []SplitPlanStage{{Name: "a", Files: []string{}}},
	}
	err := ValidateSplitPlanProposal(p)
	if err == nil || !strings.Contains(err.Error(), "files must not be empty") {
		t.Errorf("expected files error, got: %v", err)
	}
}

func TestValidateSplitPlanProposal_NegativeEstConflicts(t *testing.T) {
	p := &SplitPlanProposal{
		SessionID: "sess-1",
		Stages:    []SplitPlanStage{{Name: "a", Files: []string{"a.go"}, EstConflicts: -1}},
	}
	err := ValidateSplitPlanProposal(p)
	if err == nil || !strings.Contains(err.Error(), "estConflicts") {
		t.Errorf("expected estConflicts error, got: %v", err)
	}
}

func TestSplitPlanProposal_JSONRoundTrip(t *testing.T) {
	orig := SplitPlanProposal{
		SessionID: "sess-1",
		Stages: []SplitPlanStage{
			{Name: "types", Files: []string{"a.go", "b.go"}, Message: "add types", Order: 0, Rationale: "type defs", Independent: true, EstConflicts: 0},
			{Name: "impl", Files: []string{"c.go"}, Message: "add impl", Order: 1, EstConflicts: 2},
		},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got SplitPlanProposal
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.SessionID != orig.SessionID {
		t.Errorf("SessionID mismatch")
	}
	if len(got.Stages) != 2 {
		t.Fatalf("expected 2 stages, got %d", len(got.Stages))
	}
	if got.Stages[0].Rationale != "type defs" {
		t.Errorf("Rationale mismatch: %q", got.Stages[0].Rationale)
	}
	if !got.Stages[0].Independent {
		t.Errorf("Independent should be true")
	}
	if got.Stages[1].EstConflicts != 2 {
		t.Errorf("EstConflicts mismatch: %d", got.Stages[1].EstConflicts)
	}
}

// ---------------------------------------------------------------------------
//  ConflictReport + ConflictResolution
// ---------------------------------------------------------------------------

func TestValidateConflictReport_Valid(t *testing.T) {
	r := &ConflictReport{
		SessionID:    "sess-1",
		BranchName:   "split/types",
		VerifyOutput: "FAIL: TestFoo",
		ExitCode:     1,
		Files:        []string{"types.go"},
	}
	if err := ValidateConflictReport(r); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConflictReport_MissingBranch(t *testing.T) {
	r := &ConflictReport{
		SessionID: "sess-1",
		Files:     []string{"a.go"},
	}
	err := ValidateConflictReport(r)
	if err == nil || !strings.Contains(err.Error(), "branchName") {
		t.Errorf("expected branchName error, got: %v", err)
	}
}

func TestValidateConflictReport_EmptyFiles(t *testing.T) {
	r := &ConflictReport{
		SessionID:  "sess-1",
		BranchName: "b",
		Files:      []string{},
	}
	err := ValidateConflictReport(r)
	if err == nil || !strings.Contains(err.Error(), "files") {
		t.Errorf("expected files error, got: %v", err)
	}
}

func TestConflictReport_JSONRoundTrip(t *testing.T) {
	orig := ConflictReport{
		SessionID:    "sess-1",
		BranchName:   "split/types",
		VerifyOutput: "exit status 1",
		ExitCode:     1,
		Files:        []string{"a.go", "b.go"},
		GoModContent: "module example.com/foo",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got ConflictReport
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.BranchName != orig.BranchName {
		t.Errorf("BranchName mismatch")
	}
	if got.GoModContent != orig.GoModContent {
		t.Errorf("GoModContent mismatch")
	}
}

func TestValidateConflictResolution_Valid(t *testing.T) {
	r := &ConflictResolution{
		SessionID:  "sess-1",
		BranchName: "split/types",
		Patches:    []FilePatch{{File: "a.go", Content: "package main"}},
		Commands:   []string{"go mod tidy"},
	}
	if err := ValidateConflictResolution(r); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConflictResolution_ReSplitOnly(t *testing.T) {
	r := &ConflictResolution{
		SessionID:        "sess-1",
		BranchName:       "split/types",
		ReSplitSuggested: true,
		ReSplitReason:    "files are too coupled",
	}
	if err := ValidateConflictResolution(r); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConflictResolution_EmptyResolution(t *testing.T) {
	r := &ConflictResolution{
		SessionID:  "sess-1",
		BranchName: "split/types",
	}
	err := ValidateConflictResolution(r)
	if err == nil || !strings.Contains(err.Error(), "must include") {
		t.Errorf("expected resolution content error, got: %v", err)
	}
}

func TestValidateConflictResolution_InvalidPatchPath(t *testing.T) {
	r := &ConflictResolution{
		SessionID:  "sess-1",
		BranchName: "split/types",
		Patches:    []FilePatch{{File: "/etc/passwd", Content: "bad"}},
	}
	err := ValidateConflictResolution(r)
	if err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Errorf("expected absolute path error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
//  SteeringInstruction + InstructionAck
// ---------------------------------------------------------------------------

func TestValidateSteeringInstruction_AllTypes(t *testing.T) {
	types := []SteeringType{SteeringAbort, SteeringModifyPlan, SteeringReClassify, SteeringFocus}
	for _, st := range types {
		t.Run(string(st), func(t *testing.T) {
			i := &SteeringInstruction{SessionID: "s-1", Type: st}
			if err := ValidateSteeringInstruction(i); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateSteeringInstruction_InvalidType(t *testing.T) {
	i := &SteeringInstruction{SessionID: "s-1", Type: "invalid"}
	err := ValidateSteeringInstruction(i)
	if err == nil || !strings.Contains(err.Error(), "unknown steering type") {
		t.Errorf("expected unknown type error, got: %v", err)
	}
}

func TestValidateSteeringInstruction_MissingSessionID(t *testing.T) {
	i := &SteeringInstruction{Type: SteeringAbort}
	err := ValidateSteeringInstruction(i)
	if err == nil || !strings.Contains(err.Error(), "sessionId") {
		t.Errorf("expected sessionId error, got: %v", err)
	}
}

func TestSteeringInstruction_JSONRoundTrip(t *testing.T) {
	orig := SteeringInstruction{
		SessionID: "s-1",
		Type:      SteeringModifyPlan,
		Payload:   map[string]any{"removeStage": "docs"},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got SteeringInstruction
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Type != SteeringModifyPlan {
		t.Errorf("Type mismatch: %q", got.Type)
	}
	payload, ok := got.Payload.(map[string]any)
	if !ok {
		t.Fatalf("expected map payload, got %T", got.Payload)
	}
	if payload["removeStage"] != "docs" {
		t.Errorf("payload mismatch: %v", payload)
	}
}

func TestValidateInstructionAck_Valid(t *testing.T) {
	a := &InstructionAck{
		SessionID:       "s-1",
		InstructionType: SteeringAbort,
		Status:          AckReceived,
		Message:         "stopping",
	}
	if err := ValidateInstructionAck(a); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateInstructionAck_AllStatuses(t *testing.T) {
	statuses := []AckStatus{AckReceived, AckExecuting, AckCompleted, AckRejected}
	for _, s := range statuses {
		t.Run(string(s), func(t *testing.T) {
			a := &InstructionAck{
				SessionID:       "s-1",
				InstructionType: SteeringFocus,
				Status:          s,
			}
			if err := ValidateInstructionAck(a); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateInstructionAck_InvalidStatus(t *testing.T) {
	a := &InstructionAck{
		SessionID:       "s-1",
		InstructionType: SteeringAbort,
		Status:          "invalid",
	}
	err := ValidateInstructionAck(a)
	if err == nil || !strings.Contains(err.Error(), "unknown ack status") {
		t.Errorf("expected unknown status error, got: %v", err)
	}
}

func TestValidateInstructionAck_InvalidInstructionType(t *testing.T) {
	a := &InstructionAck{
		SessionID:       "s-1",
		InstructionType: "bad",
		Status:          AckReceived,
	}
	err := ValidateInstructionAck(a)
	if err == nil || !strings.Contains(err.Error(), "unknown instruction type") {
		t.Errorf("expected unknown instruction type error, got: %v", err)
	}
}

func TestInstructionAck_JSONRoundTrip(t *testing.T) {
	orig := InstructionAck{
		SessionID:       "s-1",
		InstructionType: SteeringReClassify,
		Status:          AckExecuting,
		Message:         "re-classifying",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got InstructionAck
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.InstructionType != SteeringReClassify {
		t.Errorf("InstructionType mismatch: %q", got.InstructionType)
	}
	if got.Status != AckExecuting {
		t.Errorf("Status mismatch: %q", got.Status)
	}
}

// ---------------------------------------------------------------------------
//  validateFilePath edge cases
// ---------------------------------------------------------------------------

func TestValidateFilePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"valid relative", "cmd/main.go", ""},
		{"valid nested", "internal/pkg/foo/bar.go", ""},
		{"empty", "", "empty path"},
		{"absolute unix", "/usr/bin/go", "absolute"},
		{"traversal", "../../etc/shadow", "traversal"},
		{"double dot in middle", "a/../b/c.go", "traversal"},
		{"unicode filename", "docs/日本語.md", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFilePath(tt.path)
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected %q error, got: %v", tt.wantErr, err)
				}
			}
		})
	}
}
