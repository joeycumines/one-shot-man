package command

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSessionPersistence_ConversationHistory(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Mock osmod with in-memory file store so savePlan/loadPlan work
	// without a real filesystem.  Also set up minimal runtime + planCache
	// so savePlan() doesn't bail early.
	_, err := evalJS(`(function() {
		var _store = {};
		osmod = {
			writeFile: function(path, data) { _store[path] = data; },
			readFile: function(path) {
				if (_store[path] !== undefined) {
					return { content: _store[path], error: null };
				}
				return { content: '', error: 'file not found: ' + path };
			}
		};
		runtime.baseBranch    = 'main';
		runtime.strategy      = 'directory';
		runtime.maxFiles      = 10;
		runtime.branchPrefix  = 'split/';
		runtime.verifyCommand = 'make';
		planCache = { splits: [{ name: 'test-split', files: ['a.go'] }] };
		return 'ok';
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	// Record 3 conversations.
	_, err = evalJS(`(function() {
		recordConversation('classify', 'what group?', 'api');
		recordConversation('plan', 'how to split?', '2 groups');
		recordConversation('resolve', 'fix conflict', 'applied patch');
		return 'ok';
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	// Verify 3 conversations recorded.
	val, err := evalJS(`getConversationHistory().length`)
	if err != nil {
		t.Fatal(err)
	}
	if val.(int64) != 3 {
		t.Fatalf("expected 3 conversations before save, got %v", val)
	}

	// Save plan (includes conversations in snapshot).
	val, err = evalJS(`JSON.stringify(savePlan())`)
	if err != nil {
		t.Fatal(err)
	}
	var saveResult struct {
		Path  string  `json:"path"`
		Error *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &saveResult); err != nil {
		t.Fatalf("parse save result: %v", err)
	}
	if saveResult.Error != nil {
		t.Fatalf("savePlan failed: %s", *saveResult.Error)
	}

	// Clear conversation history.
	_, err = evalJS(`conversationHistory = []; 'ok'`)
	if err != nil {
		t.Fatal(err)
	}
	val, err = evalJS(`getConversationHistory().length`)
	if err != nil {
		t.Fatal(err)
	}
	if val.(int64) != 0 {
		t.Fatalf("expected 0 conversations after clear, got %v", val)
	}

	// Also clear planCache so loadPlan has to restore it.
	_, err = evalJS(`planCache = null; 'ok'`)
	if err != nil {
		t.Fatal(err)
	}

	// Load plan (should restore conversations).
	val, err = evalJS(`JSON.stringify(loadPlan())`)
	if err != nil {
		t.Fatal(err)
	}
	var loadResult struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &loadResult); err != nil {
		t.Fatalf("parse load result: %v", err)
	}
	if loadResult.Error != nil {
		t.Fatalf("loadPlan failed: %s", *loadResult.Error)
	}

	// Verify 3 conversations restored.
	val, err = evalJS(`getConversationHistory().length`)
	if err != nil {
		t.Fatal(err)
	}
	if val.(int64) != 3 {
		t.Fatalf("expected 3 conversations after load, got %v", val)
	}

	// Verify content of each conversation entry.
	val, err = evalJS(`JSON.stringify(getConversationHistory())`)
	if err != nil {
		t.Fatal(err)
	}
	var convos []struct {
		Action   string `json:"action"`
		Prompt   string `json:"prompt"`
		Response string `json:"response"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &convos); err != nil {
		t.Fatalf("parse conversations: %v", err)
	}
	expected := []struct{ action, prompt, response string }{
		{"classify", "what group?", "api"},
		{"plan", "how to split?", "2 groups"},
		{"resolve", "fix conflict", "applied patch"},
	}
	for i, exp := range expected {
		if convos[i].Action != exp.action {
			t.Errorf("convo[%d].action: got %q, want %q", i, convos[i].Action, exp.action)
		}
		if convos[i].Prompt != exp.prompt {
			t.Errorf("convo[%d].prompt: got %q, want %q", i, convos[i].Prompt, exp.prompt)
		}
		if convos[i].Response != exp.response {
			t.Errorf("convo[%d].response: got %q, want %q", i, convos[i].Response, exp.response)
		}
	}
}

// ---------------------------------------------------------------------------
// T098: Verification output display via outputFn callback
// ---------------------------------------------------------------------------

func TestVerifySplit_TUIOutput(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("verifySplit uses sh -c; skipping on Windows")
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	// Create a temp git repo so verifySplit can checkout a branch.
	tmpDir := t.TempDir()

	// Set up git repo with a branch to verify.
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "init"},
		{"git", "-C", tmpDir, "symbolic-ref", "HEAD", "refs/heads/main"},
		{"git", "-C", tmpDir, "config", "user.email", "test@test.com"},
		{"git", "-C", tmpDir, "config", "user.name", "Test"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git setup %v: %v\n%s", cmd, err, out)
		}
	}
	// Create a file and commit on main.
	if err := os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "add", "."},
		{"git", "-C", tmpDir, "commit", "-m", "init"},
		{"git", "-C", tmpDir, "checkout", "-b", "split/test"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", cmd, err, out)
		}
	}
	// Add a file on the split branch.
	if err := os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "add", "."},
		{"git", "-C", tmpDir, "commit", "-m", "add b"},
		{"git", "-C", tmpDir, "checkout", "main"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", cmd, err, out)
		}
	}

	escapedDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedDir = strings.ReplaceAll(escapedDir, `'`, `\'`)

	// Call verifySplit with an outputFn that captures lines.
	val, err := evalJS(`(function() {
		var captured = [];
		var result = verifySplit('split/test', {
			dir: '` + escapedDir + `',
			verifyCommand: 'echo "line1" && echo "line2" && echo "line3"',
			outputFn: function(line) { captured.push(line); }
		});
		return JSON.stringify({ result: result, captured: captured });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var out struct {
		Result struct {
			Passed bool    `json:"passed"`
			Error  *string `json:"error"`
		} `json:"result"`
		Captured []string `json:"captured"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &out); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, val)
	}

	if !out.Result.Passed {
		errStr := ""
		if out.Result.Error != nil {
			errStr = *out.Result.Error
		}
		t.Fatalf("verify should have passed, error: %s", errStr)
	}

	// Verify output lines were captured with branch prefix.
	if len(out.Captured) < 3 {
		t.Fatalf("expected ≥3 captured lines, got %d: %v", len(out.Captured), out.Captured)
	}
	for _, line := range out.Captured {
		if !strings.Contains(line, "[verify split/test]") {
			t.Errorf("expected line to contain '[verify split/test]', got: %q", line)
		}
	}
	// Check specific content.
	foundLine1 := false
	for _, line := range out.Captured {
		if strings.Contains(line, "line1") {
			foundLine1 = true
		}
	}
	if !foundLine1 {
		t.Errorf("expected to find 'line1' in output, got: %v", out.Captured)
	}
}

// ---------------------------------------------------------------------------
// T099: Sub-step progress feedback during executeSplit
// ---------------------------------------------------------------------------

func TestExecuteSplit_ProgressFeedback(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("executeSplit uses git on filesystem; skipping on Windows for simplicity")
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tmpDir := t.TempDir()

	// Set up a git repo with many files on a source branch.
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "init"},
		{"git", "-C", tmpDir, "symbolic-ref", "HEAD", "refs/heads/main"},
		{"git", "-C", tmpDir, "config", "user.email", "test@test.com"},
		{"git", "-C", tmpDir, "config", "user.name", "Test"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git setup %v: %v\n%s", cmd, err, out)
		}
	}
	// Initial commit on main.
	if err := os.WriteFile(filepath.Join(tmpDir, "init.txt"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "add", "."},
		{"git", "-C", tmpDir, "commit", "-m", "init"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", cmd, err, out)
		}
	}

	// Create source branch with 8 files (>5 to trigger per-file progress).
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "checkout", "-b", "source"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", cmd, err, out)
		}
	}
	var allFiles []string
	for fi := 1; fi <= 8; fi++ {
		name := fmt.Sprintf("file%d.go", fi)
		allFiles = append(allFiles, name)
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(fmt.Sprintf("package f%d\n", fi)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "add", "."},
		{"git", "-C", tmpDir, "commit", "-m", "add files"},
		{"git", "-C", tmpDir, "checkout", "main"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", cmd, err, out)
		}
	}

	escapedDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedDir = strings.ReplaceAll(escapedDir, `'`, `\'`)

	// Build fileStatuses JSON.
	var statusEntries []string
	for _, f := range allFiles {
		statusEntries = append(statusEntries, fmt.Sprintf(`"%s":"A"`, f))
	}
	fileStatusesJSON := "{" + strings.Join(statusEntries, ",") + "}"

	// Build files arrays for 2 splits: first has 6 files (>5), second has 2.
	var split1Files, split2Files []string
	for i, f := range allFiles {
		if i < 6 {
			split1Files = append(split1Files, fmt.Sprintf(`"%s"`, f))
		} else {
			split2Files = append(split2Files, fmt.Sprintf(`"%s"`, f))
		}
	}

	val, err := evalJS(`(function() {
		var messages = [];
		var plan = {
			dir: '` + escapedDir + `',
			baseBranch: 'main',
			sourceBranch: 'source',
			branchPrefix: 'split/',
			fileStatuses: ` + fileStatusesJSON + `,
			splits: [
				{ name: 'split/batch-1', files: [` + strings.Join(split1Files, ",") + `], message: 'batch 1' },
				{ name: 'split/batch-2', files: [` + strings.Join(split2Files, ",") + `], message: 'batch 2' }
			]
		};
		var result = executeSplit(plan, {
			progressFn: function(msg) { messages.push(msg); }
		});
		return JSON.stringify({ error: result.error, messages: messages });
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var out struct {
		Error    *string  `json:"error"`
		Messages []string `json:"messages"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &out); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, val)
	}

	if out.Error != nil {
		t.Fatalf("executeSplit failed: %s", *out.Error)
	}

	// Expect: branch-start x2, per-file for split 1 (6 files), branch-done x2 = 2+6+2 = 10
	if len(out.Messages) < 4 {
		t.Fatalf("expected ≥4 progress messages, got %d: %v", len(out.Messages), out.Messages)
	}

	// Verify "Creating branch" messages for both splits.
	foundCreating1 := false
	foundCreating2 := false
	foundDone1 := false
	foundDone2 := false
	perFileCount := 0
	for _, msg := range out.Messages {
		if strings.Contains(msg, "Creating branch 1/2") && strings.Contains(msg, "split/batch-1") {
			foundCreating1 = true
		}
		if strings.Contains(msg, "Creating branch 2/2") && strings.Contains(msg, "split/batch-2") {
			foundCreating2 = true
		}
		if strings.Contains(msg, "Branch 1/2 created") {
			foundDone1 = true
		}
		if strings.Contains(msg, "Branch 2/2 created") {
			foundDone2 = true
		}
		if strings.Contains(msg, "file ") && strings.Contains(msg, "/") {
			perFileCount++
		}
	}

	if !foundCreating1 {
		t.Errorf("missing 'Creating branch 1/2: split/batch-1' message in: %v", out.Messages)
	}
	if !foundCreating2 {
		t.Errorf("missing 'Creating branch 2/2: split/batch-2' message in: %v", out.Messages)
	}
	if !foundDone1 {
		t.Errorf("missing 'Branch 1/2 created' message in: %v", out.Messages)
	}
	if !foundDone2 {
		t.Errorf("missing 'Branch 2/2 created' message in: %v", out.Messages)
	}
	// First split has 6 files (>5 threshold), so expect 6 per-file messages.
	if perFileCount < 6 {
		t.Errorf("expected ≥6 per-file progress messages (split 1 has 6 files >5), got %d", perFileCount)
	}
}

// ---------------------------------------------------------------------------
// T103: resolveConflicts restores branch on all exit paths
// ---------------------------------------------------------------------------

func TestResolveConflicts_RestoresBranchOnError(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("resolveConflicts uses sh -c; skipping on Windows")
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tmpDir := t.TempDir()

	// Set up git repo with main and a source branch.
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "init"},
		{"git", "-C", tmpDir, "symbolic-ref", "HEAD", "refs/heads/main"},
		{"git", "-C", tmpDir, "config", "user.email", "test@test.com"},
		{"git", "-C", tmpDir, "config", "user.name", "Test"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git setup %v: %v\n%s", cmd, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "add", "."},
		{"git", "-C", tmpDir, "commit", "-m", "init"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", cmd, err, out)
		}
	}

	escapedDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedDir = strings.ReplaceAll(escapedDir, `'`, `\'`)

	// resolveConflicts with a non-existent branch — checkout should fail,
	// but the function should still restore to the original branch.
	val, err := evalJS(`(async function() {
		var plan = {
			dir: '` + escapedDir + `',
			baseBranch: 'main',
			sourceBranch: 'main',
			splits: [
				{ name: 'nonexistent-branch', files: ['a.go'] }
			]
		};
		var result = await resolveConflicts(plan, {
			verifyCommand: 'echo ok',
			retryBudget: 1,
			perBranchRetryBudget: 1
		});
		// Check which branch we're on now.
		var currentBranch = gitExec('` + escapedDir + `', ['rev-parse', '--abbrev-ref', 'HEAD']);
		return JSON.stringify({
			errors: result.errors,
			currentBranch: currentBranch.stdout.trim()
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var out struct {
		Errors []struct {
			Error string `json:"error"`
		} `json:"errors"`
		CurrentBranch string `json:"currentBranch"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &out); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, val)
	}

	if out.CurrentBranch != "main" {
		t.Errorf("expected to be restored to 'main', got %q", out.CurrentBranch)
	}
	if len(out.Errors) == 0 {
		t.Error("expected at least one error for nonexistent branch")
	}
}

// ---------------------------------------------------------------------------
// T104: Cancellation check inside resolveConflicts strategy loop
// ---------------------------------------------------------------------------

func TestResolveConflicts_CancellationDuringStrategyLoop(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("resolveConflicts uses sh -c; skipping on Windows")
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tmpDir := t.TempDir()

	// Set up git repo with a branch that has a failing verify command.
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "init"},
		{"git", "-C", tmpDir, "symbolic-ref", "HEAD", "refs/heads/main"},
		{"git", "-C", tmpDir, "config", "user.email", "test@test.com"},
		{"git", "-C", tmpDir, "config", "user.name", "Test"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git setup %v: %v\n%s", cmd, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "a.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "add", "."},
		{"git", "-C", tmpDir, "commit", "-m", "init"},
		{"git", "-C", tmpDir, "checkout", "-b", "split/test"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", cmd, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "b.go"), []byte("package a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "add", "."},
		{"git", "-C", tmpDir, "commit", "-m", "add b"},
		{"git", "-C", tmpDir, "checkout", "main"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", cmd, err, out)
		}
	}

	escapedDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedDir = strings.ReplaceAll(escapedDir, `'`, `\'`)

	// Override isCancelled to return true immediately.
	val, err := evalJS(`(async function() {
		var origIsCancelled = isCancelled;
		isCancelled = function() { return true; };
		var plan = {
			dir: '` + escapedDir + `',
			baseBranch: 'main',
			sourceBranch: 'main',
			splits: [
				{ name: 'split/test', files: ['b.go'] }
			]
		};
		var result = await resolveConflicts(plan, {
			verifyCommand: 'exit 1',
			retryBudget: 10,
			perBranchRetryBudget: 10
		});
		isCancelled = origIsCancelled;
		// Check which branch we're on.
		var currentBranch = gitExec('` + escapedDir + `', ['rev-parse', '--abbrev-ref', 'HEAD']);
		return JSON.stringify({
			cancelledByUser: result.cancelledByUser || false,
			totalRetries: result.totalRetries,
			currentBranch: currentBranch.stdout.trim()
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var out struct {
		CancelledByUser bool   `json:"cancelledByUser"`
		TotalRetries    int    `json:"totalRetries"`
		CurrentBranch   string `json:"currentBranch"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &out); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, val)
	}

	if !out.CancelledByUser {
		t.Error("expected cancelledByUser to be true")
	}
	if out.TotalRetries != 0 {
		t.Errorf("expected 0 retries when cancelled, got %d", out.TotalRetries)
	}
	if out.CurrentBranch != "main" {
		t.Errorf("expected to be restored to 'main' after cancellation, got %q", out.CurrentBranch)
	}
}

// ---------------------------------------------------------------------------
// T105: Cancellation check inside executeSplit per-file loop
// ---------------------------------------------------------------------------

func TestExecuteSplit_CancellationMidFile(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("executeSplit uses git on filesystem; skipping on Windows")
	}

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	tmpDir := t.TempDir()

	// Set up git repo with many files on source branch.
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "init"},
		{"git", "-C", tmpDir, "symbolic-ref", "HEAD", "refs/heads/main"},
		{"git", "-C", tmpDir, "config", "user.email", "test@test.com"},
		{"git", "-C", tmpDir, "config", "user.name", "Test"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git setup %v: %v\n%s", cmd, err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "init.txt"), []byte("init\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "add", "."},
		{"git", "-C", tmpDir, "commit", "-m", "init"},
		{"git", "-C", tmpDir, "checkout", "-b", "source"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", cmd, err, out)
		}
	}

	// Create 10 files on the source branch.
	var files []string
	for fi := 1; fi <= 10; fi++ {
		name := fmt.Sprintf("f%d.go", fi)
		files = append(files, name)
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(fmt.Sprintf("package f%d\n", fi)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, cmd := range [][]string{
		{"git", "-C", tmpDir, "add", "."},
		{"git", "-C", tmpDir, "commit", "-m", "add files"},
		{"git", "-C", tmpDir, "checkout", "main"},
	} {
		out, err := exec.Command(cmd[0], cmd[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", cmd, err, out)
		}
	}

	escapedDir := strings.ReplaceAll(tmpDir, `\`, `\\`)
	escapedDir = strings.ReplaceAll(escapedDir, `'`, `\'`)

	// Build fileStatuses and files array.
	var statusEntries, fileNames []string
	for _, f := range files {
		statusEntries = append(statusEntries, fmt.Sprintf(`"%s":"A"`, f))
		fileNames = append(fileNames, fmt.Sprintf(`"%s"`, f))
	}

	// Use a per-file counter to trigger cancellation after 3 files.
	val, err := evalJS(`(function() {
		var fileCounter = 0;
		var origIsCancelled = isCancelled;
		isCancelled = function() {
			// Cancel after 3 files have been processed.
			return fileCounter++ >= 3;
		};
		var plan = {
			dir: '` + escapedDir + `',
			baseBranch: 'main',
			sourceBranch: 'source',
			branchPrefix: 'split/',
			fileStatuses: {` + strings.Join(statusEntries, ",") + `},
			splits: [
				{ name: 'split/big', files: [` + strings.Join(fileNames, ",") + `], message: 'big branch' }
			]
		};
		var result = executeSplit(plan, {});
		isCancelled = origIsCancelled;
		// Check which branch we're on.
		var currentBranch = gitExec('` + escapedDir + `', ['rev-parse', '--abbrev-ref', 'HEAD']);
		return JSON.stringify({
			error: result.error,
			currentBranch: currentBranch.stdout.trim()
		});
	})()`)
	if err != nil {
		t.Fatal(err)
	}

	var out struct {
		Error         *string `json:"error"`
		CurrentBranch string  `json:"currentBranch"`
	}
	if err := json.Unmarshal([]byte(val.(string)), &out); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, val)
	}

	if out.Error == nil {
		t.Fatal("expected error for mid-file cancellation, got nil")
	}
	if !strings.Contains(*out.Error, "cancelled by user") {
		t.Errorf("expected error to mention 'cancelled by user', got: %s", *out.Error)
	}
	if out.CurrentBranch != "main" {
		t.Errorf("expected to be restored to 'main' after cancellation, got %q", out.CurrentBranch)
	}
}

// ---------------------------------------------------------------------------
// T38: Pipeline cancellation → finishTUI resume instructions + executor cleanup
// ---------------------------------------------------------------------------

// TestAutoSplit_CancelDuringExecution_EmitsResumeAndCleansUp exercises the
// full cancellation flow: the pipeline proceeds through classification (via
// heuristic fallback), generates a plan, starts execution, then encounters
// cancellation. It verifies:
//
//  1. finishTUI emits resume instructions (plan path + osm pr-split --resume)
//  2. The mock Claude executor's close() is NOT called (heuristic fallback
//     path never spawns a process, so no process cleanup is needed)
//  3. The pipeline exits with a cancellation-related error
func TestAutoSplit_CancelDuringExecution_EmitsResumeAndCleansUp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pr-split uses sh -c; skipping on Windows")
	}

	tp := setupTestPipeline(t, TestPipelineOpts{
		ConfigOverrides: map[string]interface{}{
			"branchPrefix":  "split/",
			"verifyCommand": "true",
			"strategy":      "directory",
		},
	})

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tp.Dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	// Mock ClaudeCodeExecutor to track close() calls and fail resolve
	// (forcing heuristic fallback).
	if _, err := tp.EvalJS(`
		var _executorClosed = false;
		ClaudeCodeExecutor = function(config) { this.config = config; };
		ClaudeCodeExecutor.prototype.resolve = function() {
			return { error: 'claude not found' };
		};
		ClaudeCodeExecutor.prototype.spawn = function() {
			return { error: 'not resolved' };
		};
		ClaudeCodeExecutor.prototype.close = function() {
			_executorClosed = true;
		};
		ClaudeCodeExecutor.prototype.kill = function() {};
	`); err != nil {
		t.Fatalf("mock setup: %v", err)
	}

	// Trigger cancellation AFTER classification (heuristic) but during
	// execution. Override exec.execv to detect split branch creation and
	// set cancellation flag, simulating user pressing Ctrl+C mid-execution.
	if _, err := tp.EvalJS(`
		var _origExecv = exec.execv;
		var _cancelTriggered = false;
		exec.execv = function(cmd) {
			for (var i = 0; i < cmd.length; i++) {
				if (cmd[i] === 'checkout' && i + 1 < cmd.length && cmd[i+1] === '-b' &&
					i + 2 < cmd.length && typeof cmd[i+2] === 'string' &&
					cmd[i+2].indexOf('split/') === 0) {
					// Simulate user pressing Ctrl+C during branch creation.
					_cancelTriggered = true;
				}
			}
			return _origExecv(cmd);
		};
	`); err != nil {
		t.Fatalf("exec override: %v", err)
	}

	// Wire _cancelSource to the trigger flag for cooperative cancellation.
	if _, err := tp.EvalJS(`
		globalThis.prSplit._cancelSource = function(q) {
			if (q === 'cancelled') return _cancelTriggered;
			if (q === 'forceCancelled') return false;
			if (q === 'paused') return false;
			return false;
		};
	`); err != nil {
		t.Fatalf("cancel source mock: %v", err)
	}

	// Run the pipeline — heuristic fallback → plan → execute → cancel.
	result, err := tp.EvalJS(`JSON.stringify(await prSplit.automatedSplit({
		disableTUI: false,
		pollIntervalMs: 50,
		classifyTimeoutMs: 5000,
		planTimeoutMs: 5000,
		resolveTimeoutMs: 5000,
		maxResolveRetries: 0,
		maxReSplits: 0
	}))`)
	if err != nil {
		t.Fatalf("automatedSplit: %v", err)
	}

	var report struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(result.(string)), &report); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, result)
	}

	// Verify error indicates cancellation or execution failure.
	if report.Error == "" {
		t.Fatal("expected error from cancelled pipeline")
	}
	t.Logf("pipeline error: %s", report.Error)

	// T38 core assertions: verify stdout contains resume instructions.
	out := tp.Stdout.String()
	t.Logf("stdout:\n%s", out)

	if !strings.Contains(out, ".pr-split-plan.json") {
		t.Errorf("output should mention plan file path (.pr-split-plan.json)")
	}
	if !strings.Contains(out, "osm pr-split --resume") {
		t.Errorf("output should include resume command (osm pr-split --resume)")
	}

	// Note: executor close() is NOT called on the heuristic fallback path
	// because cleanupExecutor() is only invoked in the Claude execution
	// loop (in the pipeline chunk). When resolve() fails
	// and the pipeline falls back to heuristic mode, the executor object
	// exists but was never spawned, so no process cleanup is needed.
	closed, err := tp.EvalJS(`_executorClosed`)
	if err != nil {
		t.Fatal(err)
	}
	if closed != false {
		t.Errorf("expected executor close() NOT to be called (heuristic path), got %v", closed)
	}
}

// ---------------------------------------------------------------------------
// T114: verifySplits cancellation mid-iteration
// When isCancelled() returns true between branch verifications, verifySplits
// should return partial results with error 'verification cancelled by user'
// and restore the original branch.
// ---------------------------------------------------------------------------

func TestVerifySplits_CancellationMidIteration(t *testing.T) {
	t.Parallel()

	_, _, evalJS, _ := loadPrSplitEngineWithEval(t, nil)

	if _, err := evalJS(gitMockSetupJS()); err != nil {
		t.Fatal(err)
	}
	if _, err := evalJS(resetGitMockJS); err != nil {
		t.Fatal(err)
	}

	// Set up: 3 splits. isCancelled returns false for the first split, then
	// true before the second. The first split should complete; the second and
	// third should be skipped due to cancellation.
	if _, err := evalJS(`
		var _cancelCount = 0;
		var _origIsCancelled = isCancelled;
		isCancelled = function() {
			_cancelCount++;
			// First call (before split 0): allow
			// Second call (before split 1): cancel
			return _cancelCount >= 2;
		};

		_gitResponses['rev-parse --abbrev-ref HEAD'] = _gitOk('feature\n');
		_gitResponses['checkout'] = _gitOk('');
		_gitResponses['!sh'] = _gitOk('ok');
	`); err != nil {
		t.Fatal(err)
	}

	raw, err := evalJS(`JSON.stringify(globalThis.prSplit.verifySplits({
		dir: '/tmp/test',
		sourceBranch: 'feature',
		verifyCommand: 'make test',
		splits: [
			{name: 'split/01-first', files: ['a.go']},
			{name: 'split/02-second', files: ['b.go']},
			{name: 'split/03-third', files: ['c.go']}
		]
	}))`)
	if err != nil {
		t.Fatal(err)
	}

	// Restore isCancelled to avoid polluting subsequent tests.
	if _, err := evalJS(`isCancelled = _origIsCancelled`); err != nil {
		t.Fatal(err)
	}

	var result struct {
		AllPassed bool `json:"allPassed"`
		Results   []struct {
			Name   string  `json:"name"`
			Passed bool    `json:"passed"`
			Error  *string `json:"error"`
		} `json:"results"`
		Error *string `json:"error"`
	}
	s, ok := raw.(string)
	if !ok {
		t.Fatalf("expected string, got %T", raw)
	}
	if err := json.Unmarshal([]byte(s), &result); err != nil {
		t.Fatalf("parse: %v\nraw: %s", err, s)
	}

	// verifySplits should report not all passed.
	if result.AllPassed {
		t.Error("expected allPassed=false when cancelled mid-iteration")
	}

	// Should have exactly 1 result (the first split that completed before cancellation).
	if len(result.Results) != 1 {
		t.Errorf("expected 1 partial result (first split completed), got %d", len(result.Results))
	}

	// Top-level error should indicate cancellation.
	if result.Error == nil {
		t.Fatal("expected top-level error for cancellation")
	}
	if !strings.Contains(*result.Error, "verification cancelled by user") {
		t.Errorf("error = %q, expected 'verification cancelled by user'", *result.Error)
	}
}
