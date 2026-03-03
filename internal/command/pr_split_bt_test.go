package command

// Tests for BT (behavior tree) node factory functions and BT template
// functions exported by pr_split_script.js.
//
// T42: createAnalyzeNode, createGroupNode, createPlanNode, createSplitNode,
//      createVerifyNode, createEquivalenceNode, createSelectStrategyNode,
//      createWorkflowTree
// T43: btVerifyOutput, btRunTests, btCommitChanges, btSplitBranch,
//      verifyAndCommit
// T44: renderColorizedDiff, getSplitDiff, buildReport behavioral tests
// T45: btCommitChanges excludes .pr-split-plan.json (gitAddChangedFiles)

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
//  T42: BT Node Factory Tests
// ---------------------------------------------------------------------------

// TestBTNodeFactory_CreateAnalyzeNode_Success verifies createAnalyzeNode
// creates a blocking leaf that analyzes the diff and sets blackboard keys.
func TestBTNodeFactory_CreateAnalyzeNode_Success(t *testing.T) {
	tp := setupTestPipeline(t, TestPipelineOpts{
		FeatureFiles: []TestPipelineFile{
			{Path: "src/main.go", Content: "package main\nfunc main() {}\n"},
		},
	})

	val, err := tp.EvalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.createAnalyzeNode(bb, {
				baseBranch: prSplitConfig.baseBranch,
				dir: '` + tp.Dir + `'
			});
			var status = bt.tick(node);
			var analysis = bb.get('analysisResult');
			return JSON.stringify({
				status: status,
				hasAnalysis: !!analysis,
				fileCount: analysis ? (analysis.files || []).length : 0,
				hasError: !!bb.get('lastError')
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status      string `json:"status"`
		HasAnalysis bool   `json:"hasAnalysis"`
		FileCount   int    `json:"fileCount"`
		HasError    bool   `json:"hasError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}
	if !result.HasAnalysis {
		t.Error("expected analysisResult on blackboard")
	}
	if result.FileCount == 0 {
		t.Error("expected at least one file in analysis")
	}
	if result.HasError {
		t.Error("unexpected lastError on blackboard")
	}
}

// TestBTNodeFactory_CreateAnalyzeNode_EmptyDiff verifies createAnalyzeNode
// returns failure when no files changed.
func TestBTNodeFactory_CreateAnalyzeNode_EmptyDiff(t *testing.T) {
	tp := setupTestPipeline(t, TestPipelineOpts{
		NoFeatureFiles: true,
	})

	val, err := tp.EvalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.createAnalyzeNode(bb, {
				baseBranch: prSplitConfig.baseBranch,
				dir: '` + tp.Dir + `'
			});
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				lastError: bb.get('lastError') || ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status    string `json:"status"`
		LastError string `json:"lastError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failure" {
		t.Errorf("expected failure for empty diff, got %q", result.Status)
	}
	if result.LastError == "" {
		t.Error("expected lastError to be set")
	}
}

// TestBTNodeFactory_CreateGroupNode_Success verifies createGroupNode
// reads analysisResult from BB and writes fileGroups.
func TestBTNodeFactory_CreateGroupNode_Success(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			bb.set('analysisResult', {
				files: ['cmd/main.go', 'cmd/util.go', 'pkg/lib.go'],
				baseBranch: 'main',
				currentBranch: 'feature'
			});
			var node = prSplit.createGroupNode(bb, 'directory');
			var status = bt.tick(node);
			var groups = bb.get('fileGroups');
			return JSON.stringify({
				status: status,
				hasGroups: !!groups,
				groupCount: groups ? Object.keys(groups).length : 0
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status     string `json:"status"`
		HasGroups  bool   `json:"hasGroups"`
		GroupCount int    `json:"groupCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}
	if !result.HasGroups {
		t.Error("expected fileGroups on blackboard")
	}
	if result.GroupCount < 2 {
		t.Errorf("expected at least 2 directory groups, got %d", result.GroupCount)
	}
}

// TestBTNodeFactory_CreateGroupNode_NoAnalysis verifies createGroupNode
// fails when no analysisResult is on the blackboard.
func TestBTNodeFactory_CreateGroupNode_NoAnalysis(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.createGroupNode(bb, 'directory');
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				lastError: bb.get('lastError') || ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status    string `json:"status"`
		LastError string `json:"lastError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failure" {
		t.Errorf("expected failure, got %q", result.Status)
	}
	if !strings.Contains(result.LastError, "no analysis result") {
		t.Errorf("expected 'no analysis result' error, got %q", result.LastError)
	}
}

// TestBTNodeFactory_CreatePlanNode_Success verifies createPlanNode
// reads fileGroups from BB and writes splitPlan.
func TestBTNodeFactory_CreatePlanNode_Success(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			bb.set('analysisResult', {
				files: ['cmd/main.go', 'pkg/lib.go'],
				baseBranch: 'main',
				currentBranch: 'feature'
			});
			bb.set('fileGroups', {
				'cmd': ['cmd/main.go'],
				'pkg': ['pkg/lib.go']
			});
			var node = prSplit.createPlanNode(bb, {
				baseBranch: 'main',
				dir: '.',
				branchPrefix: 'split/',
				verifyCommand: 'true'
			});
			var status = bt.tick(node);
			var plan = bb.get('splitPlan');
			return JSON.stringify({
				status: status,
				hasPlan: !!plan,
				splitCount: plan && plan.splits ? plan.splits.length : 0
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status     string `json:"status"`
		HasPlan    bool   `json:"hasPlan"`
		SplitCount int    `json:"splitCount"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}
	if !result.HasPlan {
		t.Error("expected splitPlan on blackboard")
	}
	if result.SplitCount < 1 {
		t.Errorf("expected at least 1 split, got %d", result.SplitCount)
	}
}

// TestBTNodeFactory_CreatePlanNode_NoGroups verifies createPlanNode
// fails when fileGroups are missing.
func TestBTNodeFactory_CreatePlanNode_NoGroups(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.createPlanNode(bb, { baseBranch: 'main' });
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				lastError: bb.get('lastError') || ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status    string `json:"status"`
		LastError string `json:"lastError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failure" {
		t.Errorf("expected failure, got %q", result.Status)
	}
	if !strings.Contains(result.LastError, "no file groups") {
		t.Errorf("expected 'no file groups' error, got %q", result.LastError)
	}
}

// TestBTNodeFactory_CreateSplitNode_NoPlan verifies createSplitNode
// fails when splitPlan is missing from the blackboard.
func TestBTNodeFactory_CreateSplitNode_NoPlan(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.createSplitNode(bb);
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				lastError: bb.get('lastError') || ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status    string `json:"status"`
		LastError string `json:"lastError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failure" {
		t.Errorf("expected failure, got %q", result.Status)
	}
	if !strings.Contains(result.LastError, "no split plan") {
		t.Errorf("expected 'no split plan' error, got %q", result.LastError)
	}
}

// TestBTNodeFactory_CreateVerifyNode_NoPlan verifies createVerifyNode
// fails when splitPlan is missing from the blackboard.
func TestBTNodeFactory_CreateVerifyNode_NoPlan(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.createVerifyNode(bb);
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				lastError: bb.get('lastError') || ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status    string `json:"status"`
		LastError string `json:"lastError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failure" {
		t.Errorf("expected failure, got %q", result.Status)
	}
	if !strings.Contains(result.LastError, "no split plan") {
		t.Errorf("expected 'no split plan' error, got %q", result.LastError)
	}
}

// TestBTNodeFactory_CreateEquivalenceNode_NoPlan verifies
// createEquivalenceNode fails when splitPlan is missing.
func TestBTNodeFactory_CreateEquivalenceNode_NoPlan(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.createEquivalenceNode(bb);
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				lastError: bb.get('lastError') || ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status    string `json:"status"`
		LastError string `json:"lastError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failure" {
		t.Errorf("expected failure, got %q", result.Status)
	}
	if !strings.Contains(result.LastError, "no split plan") {
		t.Errorf("expected 'no split plan' error, got %q", result.LastError)
	}
}

// TestBTNodeFactory_CreateSelectStrategyNode_Success verifies
// createSelectStrategyNode picks a strategy and sets selectedStrategy + fileGroups.
func TestBTNodeFactory_CreateSelectStrategyNode_Success(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			bb.set('analysisResult', {
				files: ['cmd/main.go', 'cmd/util.go', 'pkg/lib.go', 'pkg/helper.go'],
				baseBranch: 'main',
				currentBranch: 'feature'
			});
			var node = prSplit.createSelectStrategyNode(bb, {});
			var status = bt.tick(node);
			var selected = bb.get('selectedStrategy');
			var groups = bb.get('fileGroups');
			return JSON.stringify({
				status: status,
				hasStrategy: !!selected,
				strategyName: selected ? (selected.name || selected.strategy || '') : '',
				hasGroups: !!groups
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status       string `json:"status"`
		HasStrategy  bool   `json:"hasStrategy"`
		StrategyName string `json:"strategyName"`
		HasGroups    bool   `json:"hasGroups"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}
	if !result.HasStrategy {
		t.Error("expected selectedStrategy on blackboard")
	}
	if !result.HasGroups {
		t.Error("expected fileGroups on blackboard")
	}
}

// TestBTNodeFactory_CreateSelectStrategyNode_NoAnalysis verifies
// failure when analysisResult is missing.
func TestBTNodeFactory_CreateSelectStrategyNode_NoAnalysis(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.createSelectStrategyNode(bb, {});
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				lastError: bb.get('lastError') || ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status    string `json:"status"`
		LastError string `json:"lastError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failure" {
		t.Errorf("expected failure, got %q", result.Status)
	}
	if !strings.Contains(result.LastError, "no analysis result") {
		t.Errorf("expected 'no analysis result' error, got %q", result.LastError)
	}
}

// TestBTNodeFactory_CreateWorkflowTree_Type verifies createWorkflowTree
// returns a valid BT node (not nil/undefined) and is a composite sequence.
func TestBTNodeFactory_CreateWorkflowTree_Type(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var tree = prSplit.createWorkflowTree(bb, {
				baseBranch: 'main',
				dir: '.',
				branchPrefix: 'split/',
				verifyCommand: 'true',
				groupStrategy: 'directory'
			});
			return JSON.stringify({
				isDefined: tree !== null && tree !== undefined,
				type: typeof tree
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		IsDefined bool   `json:"isDefined"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsDefined {
		t.Error("expected createWorkflowTree to return a defined node")
	}
	// BT composite nodes are represented as functions in Goja.
	if result.Type != "function" && result.Type != "object" {
		t.Errorf("expected node type 'function' or 'object', got %q", result.Type)
	}
}

// ---------------------------------------------------------------------------
//  T43: BT Template Function Tests
// ---------------------------------------------------------------------------

// TestBTTemplate_BtVerifyOutput_Success tests btVerifyOutput with a command
// that exits 0 and produces stdout.
func TestBTTemplate_BtVerifyOutput_Success(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.btVerifyOutput(bb, 'echo hello');
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				verified: bb.get('verified') || false,
				verifyCode: bb.get('verifyCode'),
				hasStdout: (bb.get('verifyStdout') || '').length > 0
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status    string `json:"status"`
		Verified  bool   `json:"verified"`
		Code      int    `json:"verifyCode"`
		HasStdout bool   `json:"hasStdout"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}
	if !result.Verified {
		t.Error("expected verified=true")
	}
	if result.Code != 0 {
		t.Errorf("expected exit code 0, got %d", result.Code)
	}
}

// TestBTTemplate_BtVerifyOutput_Failure tests btVerifyOutput with a failing command.
func TestBTTemplate_BtVerifyOutput_Failure(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.btVerifyOutput(bb, 'exit 1');
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				verified: bb.get('verified') || false,
				lastError: bb.get('lastError') || ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status    string `json:"status"`
		Verified  bool   `json:"verified"`
		LastError string `json:"lastError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failure" {
		t.Errorf("expected failure, got %q", result.Status)
	}
	if result.Verified {
		t.Error("expected verified=false")
	}
	if !strings.Contains(result.LastError, "verify failed") {
		t.Errorf("expected 'verify failed' error, got %q", result.LastError)
	}
}

// TestBTTemplate_BtRunTests_Success tests btRunTests with a passing command.
func TestBTTemplate_BtRunTests_Success(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.btRunTests(bb, 'echo tests pass');
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				testsPassed: bb.get('testsPassed') || false,
				testCode: bb.get('testCode')
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status      string `json:"status"`
		TestsPassed bool   `json:"testsPassed"`
		TestCode    int    `json:"testCode"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" {
		t.Errorf("expected success, got %q", result.Status)
	}
	if !result.TestsPassed {
		t.Error("expected testsPassed=true")
	}
}

// TestBTTemplate_BtRunTests_Failure tests btRunTests with a failing command.
func TestBTTemplate_BtRunTests_Failure(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.btRunTests(bb, 'exit 2');
			var status = bt.tick(node);
			return JSON.stringify({
				status: status,
				testsPassed: bb.get('testsPassed') || false,
				lastError: bb.get('lastError') || ''
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Status      string `json:"status"`
		TestsPassed bool   `json:"testsPassed"`
		LastError   string `json:"lastError"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "failure" {
		t.Errorf("expected failure, got %q", result.Status)
	}
	if result.TestsPassed {
		t.Error("expected testsPassed=false")
	}
	if !strings.Contains(result.LastError, "tests failed") {
		t.Errorf("expected 'tests failed' error, got %q", result.LastError)
	}
}

// TestBTTemplate_BtSplitBranch_NodeCreation tests btSplitBranch creates
// a valid BT node with clean blackboard preconditions.
func TestBTTemplate_BtSplitBranch_NodeCreation(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();
			var node = prSplit.btSplitBranch(bb, 'test-branch');
			return JSON.stringify({
				nodeType: typeof node,
				isDefined: node !== null && node !== undefined,
				bbClean: !bb.has('currentBranch') && !bb.has('lastError')
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		NodeType  string `json:"nodeType"`
		IsDefined bool   `json:"isDefined"`
		BBClean   bool   `json:"bbClean"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !result.IsDefined {
		t.Error("expected btSplitBranch to return a defined node")
	}
	if result.NodeType != "function" && result.NodeType != "object" {
		t.Errorf("expected node type 'function' or 'object', got %q", result.NodeType)
	}
	if !result.BBClean {
		t.Error("expected blackboard to be clean before tick")
	}
}

// TestBTTemplate_BtCommitChanges_ExcludesArtifacts verifies btCommitChanges
// stages files via gitAddChangedFiles (T45) which excludes .pr-split-plan.json.
func TestBTTemplate_BtCommitChanges_ExcludesArtifacts(t *testing.T) {
	tp := setupTestPipeline(t, TestPipelineOpts{
		FeatureFiles: []TestPipelineFile{
			{Path: "src/main.go", Content: "package main\n"},
		},
	})

	// Create a new file + the excluded artifact in the temp repo.
	testFile := filepath.Join(tp.Dir, "new-file.txt")
	if err := os.WriteFile(testFile, []byte("test content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	artifactFile := filepath.Join(tp.Dir, ".pr-split-plan.json")
	if err := os.WriteFile(artifactFile, []byte(`{"test": true}`), 0644); err != nil {
		t.Fatal(err)
	}

	val, err := tp.EvalJS(`
		(function() {
			var dir = '` + tp.Dir + `';
			var bb = new bt.Blackboard();

			// Manually invoke gitAddChangedFiles and commit via gitExec
			// to stay in the test repo dir (btCommitChanges uses CWD).
			gitAddChangedFiles(dir);

			// Check what got staged.
			var staged = prSplit._gitExec(dir, ['diff', '--cached', '--name-only']);
			var stagedFiles = staged.stdout.trim();

			// Commit.
			var commitResult = prSplit._gitExec(dir, ['commit', '-m', 'test T45 commit']);

			// Check what was committed.
			var showResult = prSplit._gitExec(dir, ['show', '--name-only', '--format=', 'HEAD']);
			var committedFiles = showResult.stdout.trim();

			return JSON.stringify({
				stagedFiles: stagedFiles,
				committedFiles: committedFiles,
				artifactExcluded: committedFiles.indexOf('.pr-split-plan.json') === -1,
				hasNewFile: committedFiles.indexOf('new-file.txt') >= 0
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		StagedFiles      string `json:"stagedFiles"`
		CommittedFiles   string `json:"committedFiles"`
		ArtifactExcluded bool   `json:"artifactExcluded"`
		HasNewFile       bool   `json:"hasNewFile"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !result.ArtifactExcluded {
		t.Errorf(".pr-split-plan.json should be excluded, staged=%s committed=%s",
			result.StagedFiles, result.CommittedFiles)
	}
	if !result.HasNewFile {
		t.Errorf("expected new-file.txt to be committed, got: %s", result.CommittedFiles)
	}
}

// TestBTTemplate_VerifyAndCommit_Type verifies verifyAndCommit returns a
// valid composite BT node (sequence of tests + optional verify + commit).
func TestBTTemplate_VerifyAndCommit_Type(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var bb = new bt.Blackboard();

			// Without verify command: tests + commit (2 nodes).
			var seq2 = prSplit.verifyAndCommit(bb, {
				testCommand: 'echo ok',
				message: 'test commit'
			});

			// With verify command: tests + verify + commit (3 nodes).
			var seq3 = prSplit.verifyAndCommit(bb, {
				testCommand: 'echo ok',
				verifyCommand: 'echo verified',
				message: 'test commit'
			});

			return JSON.stringify({
				seq2Defined: seq2 !== null && seq2 !== undefined,
				seq3Defined: seq3 !== null && seq3 !== undefined,
				seq2Type: typeof seq2,
				seq3Type: typeof seq3
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Seq2Defined bool   `json:"seq2Defined"`
		Seq3Defined bool   `json:"seq3Defined"`
		Seq2Type    string `json:"seq2Type"`
		Seq3Type    string `json:"seq3Type"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !result.Seq2Defined {
		t.Error("expected verifyAndCommit (no verify) to return a node")
	}
	if !result.Seq3Defined {
		t.Error("expected verifyAndCommit (with verify) to return a node")
	}
}

// ---------------------------------------------------------------------------
//  T81: verifyAndCommit BT Execution (tick-level verification)
// ---------------------------------------------------------------------------

// TestVerifyAndCommit_BTExecution ticks verifyAndCommit sequences in a real
// git repo to verify the BT nodes execute test, verify, and commit steps.
// Uses os.Chdir — must NOT be t.Parallel.
func TestVerifyAndCommit_BTExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	repoDir := initIntegrationRepo(t)

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repoDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	t.Run("with_verify_command", func(t *testing.T) {
		// Create a file change that can be committed.
		if err := os.WriteFile(filepath.Join(repoDir, "verify-test.txt"), []byte("verify content\n"), 0644); err != nil {
			t.Fatal(err)
		}

		_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

		val, err := evalJS(`
			(function() {
				var bb = new bt.Blackboard();
				var node = prSplit.verifyAndCommit(bb, {
					testCommand: 'echo tests-pass',
					verifyCommand: 'echo verified-ok',
					message: 'commit with verify'
				});
				var status = bt.tick(node);
				return JSON.stringify({
					status: status,
					testsPassed: bb.get('testsPassed') || false,
					verified: bb.get('verified') || false,
					committed: bb.get('committed') || false,
					lastError: bb.get('lastError') || ''
				});
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Status      string `json:"status"`
			TestsPassed bool   `json:"testsPassed"`
			Verified    bool   `json:"verified"`
			Committed   bool   `json:"committed"`
			LastError   string `json:"lastError"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
			t.Fatal(err)
		}
		if result.Status != "success" {
			t.Errorf("expected success, got %q (error: %s)", result.Status, result.LastError)
		}
		if !result.TestsPassed {
			t.Error("expected testsPassed=true")
		}
		if !result.Verified {
			t.Error("expected verified=true (verifyCommand should set verified)")
		}
		if !result.Committed {
			t.Error("expected committed=true")
		}
	})

	t.Run("test_only_path", func(t *testing.T) {
		// Create a new file to commit.
		if err := os.WriteFile(filepath.Join(repoDir, "test-only.txt"), []byte("test only content\n"), 0644); err != nil {
			t.Fatal(err)
		}

		_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

		val, err := evalJS(`
			(function() {
				var bb = new bt.Blackboard();
				var node = prSplit.verifyAndCommit(bb, {
					testCommand: 'echo tests-pass',
					message: 'commit without verify'
				});
				var status = bt.tick(node);
				return JSON.stringify({
					status: status,
					testsPassed: bb.get('testsPassed') || false,
					verified: bb.get('verified') || false,
					committed: bb.get('committed') || false,
					lastError: bb.get('lastError') || ''
				});
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Status      string `json:"status"`
			TestsPassed bool   `json:"testsPassed"`
			Verified    bool   `json:"verified"`
			Committed   bool   `json:"committed"`
			LastError   string `json:"lastError"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
			t.Fatal(err)
		}
		if result.Status != "success" {
			t.Errorf("expected success, got %q (error: %s)", result.Status, result.LastError)
		}
		if !result.TestsPassed {
			t.Error("expected testsPassed=true")
		}
		if result.Verified {
			t.Error("expected verified=false (no verifyCommand)")
		}
		if !result.Committed {
			t.Error("expected committed=true")
		}
	})

	t.Run("test_failure_aborts_sequence", func(t *testing.T) {
		// Create a file — but test will fail, so it shouldn't be committed.
		if err := os.WriteFile(filepath.Join(repoDir, "not-committed.txt"), []byte("should not commit\n"), 0644); err != nil {
			t.Fatal(err)
		}

		_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

		val, err := evalJS(`
			(function() {
				var bb = new bt.Blackboard();
				var node = prSplit.verifyAndCommit(bb, {
					testCommand: 'exit 1',
					verifyCommand: 'echo should-not-run',
					message: 'should not commit'
				});
				var status = bt.tick(node);
				return JSON.stringify({
					status: status,
					testsPassed: bb.get('testsPassed') || false,
					verified: bb.get('verified') || false,
					committed: bb.get('committed') || false,
					lastError: bb.get('lastError') || ''
				});
			})()
		`)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Status      string `json:"status"`
			TestsPassed bool   `json:"testsPassed"`
			Verified    bool   `json:"verified"`
			Committed   bool   `json:"committed"`
			LastError   string `json:"lastError"`
		}
		if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
			t.Fatal(err)
		}
		if result.Status != "failure" {
			t.Errorf("expected failure, got %q", result.Status)
		}
		if result.TestsPassed {
			t.Error("expected testsPassed=false")
		}
		if result.Verified {
			t.Error("expected verified=false (sequence should abort before verify)")
		}
		if result.Committed {
			t.Error("expected committed=false (sequence should abort before commit)")
		}
		if !strings.Contains(result.LastError, "tests failed") {
			t.Errorf("expected 'tests failed' error, got %q", result.LastError)
		}
	})
}

// ---------------------------------------------------------------------------
//  T44: Behavioral Tests for renderColorizedDiff, getSplitDiff, buildReport
// ---------------------------------------------------------------------------

// TestRenderColorizedDiff_ANSICodes verifies renderColorizedDiff produces
// ANSI-colored output for additions, deletions, hunks, and context.
func TestRenderColorizedDiff_ANSICodes(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`
		(function() {
			var diff = [
				'diff --git a/foo.go b/foo.go',
				'index abc123..def456 100644',
				'--- a/foo.go',
				'+++ b/foo.go',
				'@@ -1,3 +1,4 @@',
				' package foo',
				'-func old() {}',
				'+func new() {}',
				'+func extra() {}',
				' // end'
			].join('\n');
			var result = prSplit.renderColorizedDiff(diff);
			return JSON.stringify({
				hasGreen: result.indexOf('\x1b[32m') >= 0,
				hasRed: result.indexOf('\x1b[31m') >= 0,
				hasCyan: result.indexOf('\x1b[36m') >= 0,
				hasBold: result.indexOf('\x1b[1m') >= 0,
				hasReset: result.indexOf('\x1b[0m') >= 0,
				lineCount: result.split('\n').length,
				notEmpty: result.length > 0
			});
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		HasGreen  bool `json:"hasGreen"`
		HasRed    bool `json:"hasRed"`
		HasCyan   bool `json:"hasCyan"`
		HasBold   bool `json:"hasBold"`
		HasReset  bool `json:"hasReset"`
		LineCount int  `json:"lineCount"`
		NotEmpty  bool `json:"notEmpty"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !result.HasGreen {
		t.Error("expected green ANSI code for additions")
	}
	if !result.HasRed {
		t.Error("expected red ANSI code for deletions")
	}
	if !result.HasCyan {
		t.Error("expected cyan ANSI code for hunk headers")
	}
	if !result.HasBold {
		t.Error("expected bold ANSI code for diff/index/---/+++ lines")
	}
	if !result.HasReset {
		t.Error("expected reset ANSI codes")
	}
	if result.LineCount != 10 {
		t.Errorf("expected 10 lines, got %d", result.LineCount)
	}
}

// TestRenderColorizedDiff_EmptyInput verifies empty input returns empty string.
func TestRenderColorizedDiff_EmptyInput(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`prSplit.renderColorizedDiff('')`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("expected empty string for empty input, got %q", val)
	}
}

// TestGetSplitDiff_InvalidIndex verifies getSplitDiff returns error for
// out-of-bounds split index.
func TestGetSplitDiff_InvalidIndex(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({ splits: [{ name: 'a', files: ['f.go'] }] }, 5))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error string `json:"error"`
		Diff  string `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == "" {
		t.Error("expected error for invalid split index")
	}
	if !strings.Contains(result.Error, "invalid split index") {
		t.Errorf("expected 'invalid split index' error, got %q", result.Error)
	}
}

// TestGetSplitDiff_EmptyFiles verifies getSplitDiff handles split with no files.
func TestGetSplitDiff_EmptyFiles(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({ splits: [{ name: 'a', files: [] }] }, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error string `json:"error"`
		Diff  string `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == "" {
		t.Error("expected error for empty files")
	}
	if !strings.Contains(result.Error, "no files") {
		t.Errorf("expected 'no files' error, got %q", result.Error)
	}
}

// TestGetSplitDiff_NullPlan verifies getSplitDiff handles null/undefined plan.
func TestGetSplitDiff_NullPlan(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff(null, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error string `json:"error"`
		Diff  string `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == "" {
		t.Error("expected error for null plan")
	}
}

// TestGetSplitDiff_NegativeIndex verifies getSplitDiff handles negative index.
func TestGetSplitDiff_NegativeIndex(t *testing.T) {
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({ splits: [{ name: 'a', files: ['f.go'] }] }, -1))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error == "" {
		t.Error("expected error for negative index")
	}
}

// ---------------------------------------------------------------------------
// T80: getSplitDiff success + fallback path tests
// ---------------------------------------------------------------------------

// TestGetSplitDiff_SuccessWithDiff verifies getSplitDiff returns diff content
// when the primary git diff (baseBranch...splitName) succeeds.
func TestGetSplitDiff_SuccessWithDiff(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Mock: primary diff returns content.
	if _, err := evalJS(`
		globalThis._gitResponses['diff'] = _gitOk('diff --git a/f.go b/f.go\n+new line\n');
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({
		baseBranch: 'main',
		splits: [{name: 'split/01', files: ['f.go']}]
	}, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error *string `json:"error"`
		Diff  string  `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error != nil {
		t.Errorf("expected no error, got: %s", *result.Error)
	}
	if !strings.Contains(result.Diff, "+new line") {
		t.Errorf("expected diff content, got: %q", result.Diff)
	}
}

// TestGetSplitDiff_FallbackOnThreeDotFailure verifies that when the primary
// git diff (baseBranch...splitName) fails, getSplitDiff falls back to
// git diff baseBranch -- files.
func TestGetSplitDiff_FallbackOnThreeDotFailure(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Mock: primary diff (baseBranch...splitName) fails.
	// Fallback diff (baseBranch -- files) succeeds.
	if _, err := evalJS(`
		var diffCallCount = 0;
		globalThis._gitResponses['diff'] = function(argv) {
			diffCallCount++;
			// First call: three-dot diff — fail.
			if (diffCallCount === 1) {
				return _gitFail('unknown revision');
			}
			// Second call: fallback — succeed.
			return _gitOk('diff --git a/f.go b/f.go\n+fallback content\n');
		};
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({
		baseBranch: 'main',
		splits: [{name: 'split/01', files: ['f.go']}]
	}, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error *string `json:"error"`
		Diff  string  `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Error != nil {
		t.Errorf("expected no error (fallback success), got: %s", *result.Error)
	}
	if !strings.Contains(result.Diff, "+fallback content") {
		t.Errorf("expected fallback diff content, got: %q", result.Diff)
	}

	// Verify fallback was used (2 diff calls).
	countVal, err := evalJS(`diffCallCount`)
	if err != nil {
		t.Fatal(err)
	}
	if countVal.(int64) != 2 {
		t.Errorf("expected 2 diff calls (primary + fallback), got %d", countVal.(int64))
	}
}

// TestGetSplitDiff_BothDiffsFail verifies getSplitDiff returns error when
// both primary and fallback diffs fail.
func TestGetSplitDiff_BothDiffsFail(t *testing.T) {
	t.Parallel()
	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}

	// Mock: both diffs fail.
	if _, err := evalJS(`
		globalThis._gitResponses['diff'] = _gitFail('fatal: bad object');
	`); err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(prSplit.getSplitDiff({
		baseBranch: 'main',
		splits: [{name: 'split/01', files: ['f.go']}]
	}, 0))`)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Error string `json:"error"`
		Diff  string `json:"diff"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Error, "git diff failed") {
		t.Errorf("expected 'git diff failed' error, got: %q", result.Error)
	}
}

// TestBuildReport_WithNullCaches verifies the 'report' command outputs valid
// JSON when no caches are populated (no analyze/group/plan has been run).
func TestBuildReport_WithNullCaches(t *testing.T) {
	tp := setupTestPipeline(t, TestPipelineOpts{
		FeatureFiles: []TestPipelineFile{
			{Path: "src/main.go", Content: "package main\n"},
		},
	})

	// Dispatch 'report' without running analyze first.
	if err := tp.Dispatch("report", nil); err != nil {
		t.Fatal(err)
	}

	out := tp.Stdout.String()
	// Find the JSON object in the output (may have other prints).
	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("expected JSON in output, got: %s", out)
	}
	jsonStr := out[idx:]
	// Find matching closing brace.
	depth := 0
	end := -1
	for i, c := range jsonStr {
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if end < 0 {
		t.Fatalf("incomplete JSON in output: %s", jsonStr)
	}
	jsonStr = jsonStr[:end]

	var report map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
		t.Fatalf("failed to parse report JSON: %v\nraw: %s", err, jsonStr)
	}
	if _, ok := report["version"]; !ok {
		t.Error("expected report to have 'version' field")
	}
	if report["analysis"] != nil {
		t.Error("expected report.analysis to be null when no analyze has been run")
	}
	if report["groups"] != nil {
		t.Error("expected report.groups to be null when no group has been run")
	}
	if report["plan"] != nil {
		t.Error("expected report.plan to be null when no plan has been run")
	}
}

// TestBuildReport_WithPopulatedCaches verifies the 'report' command produces
// complete JSON when analyze, group, and plan have been run.
func TestBuildReport_WithPopulatedCaches(t *testing.T) {
	tp := setupTestPipeline(t, TestPipelineOpts{
		FeatureFiles: []TestPipelineFile{
			{Path: "cmd/run.go", Content: "package main\n\nfunc run() {}\n"},
			{Path: "pkg/lib.go", Content: "package pkg\n\nfunc Lib() {}\n"},
		},
	})

	// Run analyze → group → plan → report through dispatch.
	for _, cmd := range []string{"analyze", "group", "plan"} {
		if err := tp.Dispatch(cmd, nil); err != nil {
			t.Fatalf("dispatch %q failed: %v", cmd, err)
		}
	}

	// Clear stdout before report capture.
	tp.Stdout.Reset()
	if err := tp.Dispatch("report", nil); err != nil {
		t.Fatal(err)
	}

	out := tp.Stdout.String()
	idx := strings.Index(out, "{")
	if idx < 0 {
		t.Fatalf("expected JSON in output, got: %s", out)
	}
	jsonStr := out[idx:]
	depth := 0
	end := -1
	for i, c := range jsonStr {
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}
	if end < 0 {
		t.Fatalf("incomplete JSON in output: %s", jsonStr)
	}
	jsonStr = jsonStr[:end]

	var report map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &report); err != nil {
		t.Fatalf("failed to parse report JSON: %v\nraw: %s", err, jsonStr)
	}

	// Version
	version, _ := report["version"].(string)
	if version == "" || version == "unknown" {
		t.Errorf("expected valid version, got %q", version)
	}

	// Analysis
	analysis, _ := report["analysis"].(map[string]interface{})
	if analysis == nil {
		t.Fatal("expected report.analysis to be populated after analyze")
	}
	fileCount, _ := analysis["fileCount"].(float64)
	if fileCount < 1 {
		t.Errorf("expected at least 1 file in analysis, got %v", fileCount)
	}

	// Groups
	groups, _ := report["groups"].([]interface{})
	if len(groups) == 0 {
		t.Error("expected report.groups to be populated after group")
	}

	// Plan
	plan, _ := report["plan"].(map[string]interface{})
	if plan == nil {
		t.Fatal("expected report.plan to be populated after plan")
	}
	splitCount, _ := plan["splitCount"].(float64)
	if splitCount < 1 {
		t.Errorf("expected at least 1 split in plan, got %v", splitCount)
	}
}
