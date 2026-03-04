package claudemux

import (
	"sync"
	"testing"
)

func TestIntentName(t *testing.T) {
	tests := []struct {
		intent Intent
		want   string
	}{
		{IntentUnknown, "Unknown"},
		{IntentReadOnly, "ReadOnly"},
		{IntentCode, "Code"},
		{IntentDestructive, "Destructive"},
		{IntentNetwork, "Network"},
		{IntentCredential, "Credential"},
		{Intent(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := IntentName(tt.intent); got != tt.want {
			t.Errorf("IntentName(%d) = %q, want %q", tt.intent, got, tt.want)
		}
	}
}

func TestScopeName(t *testing.T) {
	tests := []struct {
		scope Scope
		want  string
	}{
		{ScopeUnknown, "Unknown"},
		{ScopeFile, "File"},
		{ScopeRepo, "Repo"},
		{ScopeInfra, "Infra"},
		{Scope(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := ScopeName(tt.scope); got != tt.want {
			t.Errorf("ScopeName(%d) = %q, want %q", tt.scope, got, tt.want)
		}
	}
}

func TestRiskLevelName(t *testing.T) {
	tests := []struct {
		level RiskLevel
		want  string
	}{
		{RiskNone, "None"},
		{RiskLow, "Low"},
		{RiskMedium, "Medium"},
		{RiskHigh, "High"},
		{RiskCritical, "Critical"},
		{RiskLevel(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := RiskLevelName(tt.level); got != tt.want {
			t.Errorf("RiskLevelName(%d) = %q, want %q", tt.level, got, tt.want)
		}
	}
}

func TestPolicyActionName(t *testing.T) {
	tests := []struct {
		action PolicyAction
		want   string
	}{
		{PolicyAllow, "Allow"},
		{PolicyWarn, "Warn"},
		{PolicyConfirm, "Confirm"},
		{PolicyBlock, "Block"},
		{PolicyAction(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := PolicyActionName(tt.action); got != tt.want {
			t.Errorf("PolicyActionName(%d) = %q, want %q", tt.action, got, tt.want)
		}
	}
}

func TestDefaultSafetyConfig(t *testing.T) {
	cfg := DefaultSafetyConfig()
	if !cfg.Enabled {
		t.Error("default config should be enabled")
	}
	if cfg.DefaultAction != PolicyAllow {
		t.Errorf("default action = %v, want PolicyAllow", cfg.DefaultAction)
	}
	if cfg.WarnThreshold != 0.3 {
		t.Errorf("warn threshold = %v, want 0.3", cfg.WarnThreshold)
	}
	if cfg.ConfirmThreshold != 0.6 {
		t.Errorf("confirm threshold = %v, want 0.6", cfg.ConfirmThreshold)
	}
	if cfg.BlockThreshold != 0.9 {
		t.Errorf("block threshold = %v, want 0.9", cfg.BlockThreshold)
	}
	if len(cfg.SensitivePatterns) == 0 {
		t.Error("default config should have sensitive patterns")
	}
}

func TestNewSafetyValidator(t *testing.T) {
	cfg := DefaultSafetyConfig()
	sv := NewSafetyValidator(cfg)
	if sv == nil {
		t.Fatal("NewSafetyValidator returned nil")
	}
	if !sv.Config().Enabled {
		t.Error("validator should be enabled")
	}
}

func TestSafetyValidator_Disabled(t *testing.T) {
	cfg := DefaultSafetyConfig()
	cfg.Enabled = false
	sv := NewSafetyValidator(cfg)

	a := sv.Validate(SafetyAction{
		Type: "command",
		Name: "rm",
		Raw:  "rm -rf /",
	})

	if a.Action != PolicyAllow {
		t.Errorf("disabled validator should allow, got %v", PolicyActionName(a.Action))
	}
	if a.RiskScore != 0 {
		t.Errorf("disabled validator risk should be 0, got %v", a.RiskScore)
	}
}

func TestSafetyValidator_ReadOnly(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type:      "file_read",
		Name:      "readFile",
		FilePaths: []string{"src/main.go"},
	})

	if a.Intent != IntentReadOnly {
		t.Errorf("intent = %v, want ReadOnly", IntentName(a.Intent))
	}
	if a.Action != PolicyAllow {
		t.Errorf("read-only should be allowed, got %v", PolicyActionName(a.Action))
	}
	if a.RiskScore > 0.1 {
		t.Errorf("read-only risk = %v, expected < 0.1", a.RiskScore)
	}
}

func TestSafetyValidator_CodeModification(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type:      "file_write",
		Name:      "writeFile",
		FilePaths: []string{"src/main.go"},
	})

	if a.Intent != IntentCode {
		t.Errorf("intent = %v, want Code", IntentName(a.Intent))
	}
	if a.Scope != ScopeFile {
		t.Errorf("scope = %v, want File", ScopeName(a.Scope))
	}
}

func TestSafetyValidator_DestructiveCommand(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type: "command",
		Name: "exec",
		Raw:  "rm -rf /tmp/build",
	})

	if a.Intent != IntentDestructive {
		t.Errorf("intent = %v, want Destructive", IntentName(a.Intent))
	}
	if a.RiskScore < 0.5 {
		t.Errorf("destructive risk = %v, expected >= 0.5", a.RiskScore)
	}
}

func TestSafetyValidator_NetworkAccess(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type: "command",
		Name: "exec",
		Raw:  "curl https://api.example.com/data",
	})

	if a.Intent != IntentNetwork {
		t.Errorf("intent = %v, want Network", IntentName(a.Intent))
	}
	if a.Action == PolicyAllow {
		t.Error("network access should not be plain Allow with default thresholds")
	}
}

func TestSafetyValidator_CredentialAccess(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type:      "file_read",
		Name:      "readFile",
		FilePaths: []string{"/home/user/.ssh/id_rsa"},
	})

	if a.Intent != IntentCredential {
		t.Errorf("intent = %v, want Credential", IntentName(a.Intent))
	}
	if a.RiskScore < 0.5 {
		t.Errorf("credential risk = %v, expected >= 0.5", a.RiskScore)
	}
}

func TestSafetyValidator_SensitivePattern(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type: "tool_call",
		Name: "editConfig",
		Args: map[string]string{"key": "api_key", "value": "sk-12345"},
	})

	if a.Intent != IntentCredential {
		t.Errorf("intent = %v, want Credential", IntentName(a.Intent))
	}
}

func TestSafetyValidator_BlockedTool(t *testing.T) {
	cfg := DefaultSafetyConfig()
	cfg.BlockedTools = []string{"dangerousTool", "shellExec"}
	sv := NewSafetyValidator(cfg)

	a := sv.Validate(SafetyAction{
		Name: "dangerousTool",
	})

	if a.Action != PolicyBlock {
		t.Errorf("blocked tool should be blocked, got %v", PolicyActionName(a.Action))
	}
	if a.RiskScore != 1.0 {
		t.Errorf("blocked tool risk = %v, want 1.0", a.RiskScore)
	}
	if a.Details["tool"] != "dangerousTool" {
		t.Errorf("details.tool = %q, want %q", a.Details["tool"], "dangerousTool")
	}
}

func TestSafetyValidator_BlockedTool_Allowed(t *testing.T) {
	cfg := DefaultSafetyConfig()
	cfg.BlockedTools = []string{"dangerousTool"}
	sv := NewSafetyValidator(cfg)

	a := sv.Validate(SafetyAction{
		Name: "safeTool",
		Type: "file_read",
	})

	if a.Action == PolicyBlock {
		t.Error("non-blocked tool should not be blocked")
	}
}

func TestSafetyValidator_BlockedPath(t *testing.T) {
	cfg := DefaultSafetyConfig()
	cfg.BlockedPaths = []string{"*.pem", "/etc/*"}
	sv := NewSafetyValidator(cfg)

	a := sv.Validate(SafetyAction{
		Name:      "writeFile",
		FilePaths: []string{"server.pem"},
	})

	if a.Action != PolicyBlock {
		t.Errorf("blocked path should be blocked, got %v", PolicyActionName(a.Action))
	}
}

func TestSafetyValidator_AllowedPaths(t *testing.T) {
	cfg := DefaultSafetyConfig()
	cfg.AllowedPaths = []string{"src/*", "tests/*"}
	sv := NewSafetyValidator(cfg)

	// Allowed path.
	a := sv.Validate(SafetyAction{
		Type:      "file_write",
		Name:      "writeFile",
		FilePaths: []string{"src/main.go"},
	})
	if a.Action == PolicyBlock {
		t.Error("path in allowlist should not be blocked")
	}

	// Not-allowed path.
	a = sv.Validate(SafetyAction{
		Type:      "file_write",
		Name:      "writeFile",
		FilePaths: []string{"secret/config.yml"},
	})
	if a.Action != PolicyBlock {
		t.Errorf("path not in allowlist should be blocked, got %v", PolicyActionName(a.Action))
	}
}

func TestSafetyValidator_InfraScope(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type: "command",
		Name: "exec",
		Raw:  "sudo systemctl restart nginx",
	})

	if a.Scope != ScopeInfra {
		t.Errorf("scope = %v, want Infra", ScopeName(a.Scope))
	}
}

func TestSafetyValidator_RepoScope(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type: "command",
		Name: "exec",
		Raw:  "git push --force origin main",
	})

	if a.Scope != ScopeRepo {
		t.Errorf("scope = %v, want Repo", ScopeName(a.Scope))
	}
}

func TestSafetyValidator_MultipleFilePaths_RepoScope(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type:      "file_write",
		Name:      "writeFile",
		FilePaths: []string{"a.go", "b.go", "c.go", "d.go"},
	})

	if a.Scope != ScopeRepo {
		t.Errorf("4+ file paths scope = %v, want Repo", ScopeName(a.Scope))
	}
}

func TestSafetyValidator_SystemPath_InfraScope(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type:      "file_write",
		Name:      "writeFile",
		FilePaths: []string{"/etc/hosts"},
	})

	if a.Scope != ScopeInfra {
		t.Errorf("system path scope = %v, want Infra", ScopeName(a.Scope))
	}
}

func TestSafetyValidator_RiskScoring(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	// Low risk: read-only, file scope.
	readA := sv.Validate(SafetyAction{
		Type:      "file_read",
		Name:      "readFile",
		FilePaths: []string{"README.md"},
	})

	// High risk: destructive, infra scope.
	destructA := sv.Validate(SafetyAction{
		Type: "command",
		Name: "exec",
		Raw:  "sudo rm -rf /var/data",
	})

	if readA.RiskScore >= destructA.RiskScore {
		t.Errorf("read risk (%v) should be less than destructive risk (%v)",
			readA.RiskScore, destructA.RiskScore)
	}
}

func TestSafetyValidator_PolicyEnforcement(t *testing.T) {
	cfg := DefaultSafetyConfig()
	cfg.WarnThreshold = 0.2
	cfg.ConfirmThreshold = 0.5
	cfg.BlockThreshold = 0.8
	sv := NewSafetyValidator(cfg)

	// Low risk → allow.
	low := sv.Validate(SafetyAction{
		Type:      "file_read",
		Name:      "readFile",
		FilePaths: []string{"main.go"},
	})
	if low.Action != PolicyAllow {
		t.Errorf("low risk action = %v, want Allow", PolicyActionName(low.Action))
	}

	// High risk → confirm or block.
	high := sv.Validate(SafetyAction{
		Type: "command",
		Name: "exec",
		Raw:  "rm -rf /important",
	})
	if high.Action == PolicyAllow {
		t.Error("high risk action should not be Allow")
	}
}

func TestSafetyValidator_Stats(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	sv.Validate(SafetyAction{Type: "file_read", Name: "readFile", FilePaths: []string{"a.go"}})
	sv.Validate(SafetyAction{Type: "file_read", Name: "readFile", FilePaths: []string{"b.go"}})
	sv.Validate(SafetyAction{Type: "command", Name: "exec", Raw: "rm -rf /tmp"})

	stats := sv.Stats()
	if stats.TotalChecks != 3 {
		t.Errorf("total checks = %d, want 3", stats.TotalChecks)
	}
	if stats.IntentCounts["ReadOnly"] != 2 {
		t.Errorf("ReadOnly count = %d, want 2", stats.IntentCounts["ReadOnly"])
	}
}

func TestSafetyValidator_ToolNameHeuristic_Write(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Name: "writeConfig",
	})

	if a.Intent != IntentCode {
		t.Errorf("intent = %v, want Code (tool name heuristic)", IntentName(a.Intent))
	}
}

func TestSafetyValidator_ToolNameHeuristic_Delete(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Name: "deleteUser",
	})

	if a.Intent != IntentDestructive {
		t.Errorf("intent = %v, want Destructive (tool name heuristic)", IntentName(a.Intent))
	}
}

func TestSafetyValidator_ToolNameHeuristic_Read(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Name: "getStatus",
	})

	if a.Intent != IntentReadOnly {
		t.Errorf("intent = %v, want ReadOnly (tool name heuristic)", IntentName(a.Intent))
	}
}

func TestSafetyValidator_UnknownIntent(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Name: "foobar",
	})

	if a.Intent != IntentUnknown {
		t.Errorf("intent = %v, want Unknown", IntentName(a.Intent))
	}
}

func TestSafetyValidator_DestructiveInfra_HighRisk(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type: "command",
		Name: "exec",
		Raw:  "sudo rm -rf /etc/nginx",
	})

	if a.Intent != IntentDestructive {
		t.Errorf("intent = %v, want Destructive", IntentName(a.Intent))
	}
	if a.Scope != ScopeInfra {
		t.Errorf("scope = %v, want Infra", ScopeName(a.Scope))
	}
	if a.RiskLevel < RiskHigh {
		t.Errorf("risk level = %v, want >= High", RiskLevelName(a.RiskLevel))
	}
}

func TestSafetyValidator_CredentialWithSensitiveArgs(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type: "tool_call",
		Name: "setEnv",
		Args: map[string]string{"name": "SECRET_TOKEN", "value": "abc123"},
	})

	// The sensitive pattern should match "SECRET_TOKEN" and classify as credential.
	if a.Intent != IntentCredential {
		t.Errorf("intent = %v, want Credential", IntentName(a.Intent))
	}
}

// TestSafetyValidator_FileDeleteType verifies file_delete type is classified as destructive.
func TestSafetyValidator_FileDeleteType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type:      "file_delete",
		Name:      "deleteFile",
		FilePaths: []string{"old.go"},
	})

	if a.Intent != IntentDestructive {
		t.Errorf("intent = %v, want Destructive", IntentName(a.Intent))
	}
}

func TestSafetyValidator_FileCreateType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type:      "file_create",
		Name:      "createFile",
		FilePaths: []string{"new.go"},
	})

	if a.Intent != IntentCode {
		t.Errorf("intent = %v, want Code", IntentName(a.Intent))
	}
}

func TestSafetyValidator_SearchType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type: "search",
		Name: "grepSearch",
	})

	if a.Intent != IntentReadOnly {
		t.Errorf("intent = %v, want ReadOnly", IntentName(a.Intent))
	}
}

func TestSafetyValidator_NetworkType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type: "network",
		Name: "httpClient",
	})

	if a.Intent != IntentNetwork {
		t.Errorf("intent = %v, want Network", IntentName(a.Intent))
	}
}

// TestSafetyValidator_InvalidSensitivePattern verifies graceful handling of bad regex.
func TestSafetyValidator_InvalidSensitivePattern(t *testing.T) {
	cfg := DefaultSafetyConfig()
	cfg.SensitivePatterns = []string{"[invalid", `valid_pattern`}
	sv := NewSafetyValidator(cfg)

	// Should not panic.
	a := sv.Validate(SafetyAction{
		Type: "file_read",
		Name: "readFile",
	})

	if a.Action == PolicyBlock {
		t.Error("invalid pattern should not cause blocking")
	}
	// Only the valid pattern should be compiled.
	if len(sv.sensitivePatterns) != 1 {
		t.Errorf("sensitive patterns = %d, want 1 (invalid skipped)", len(sv.sensitivePatterns))
	}
}

func TestRiskLevelFromScore(t *testing.T) {
	tests := []struct {
		score float64
		want  RiskLevel
	}{
		{0.0, RiskNone},
		{0.05, RiskNone},
		{0.06, RiskLow},
		{0.29, RiskLow},
		{0.3, RiskMedium},
		{0.59, RiskMedium},
		{0.6, RiskHigh},
		{0.79, RiskHigh},
		{0.8, RiskCritical},
		{1.0, RiskCritical},
	}
	for _, tt := range tests {
		if got := riskLevelFromScore(tt.score); got != tt.want {
			t.Errorf("riskLevelFromScore(%v) = %v, want %v", tt.score, RiskLevelName(got), RiskLevelName(tt.want))
		}
	}
}

func TestBuildSearchText(t *testing.T) {
	action := SafetyAction{
		Type:      "command",
		Name:      "exec",
		Raw:       "ls -la",
		FilePaths: []string{"/tmp"},
		Args:      map[string]string{"shell": "bash"},
	}
	text := buildSearchText(action)
	if text == "" {
		t.Error("search text should not be empty")
	}
	for _, want := range []string{"command", "exec", "ls -la", "/tmp", "shell=bash"} {
		if !containsStr(text, want) {
			t.Errorf("search text %q missing %q", text, want)
		}
	}
}

func TestBuildReason(t *testing.T) {
	r := buildReason(IntentCode, ScopeFile, RiskLow, PolicyAllow)
	if r == "" {
		t.Error("reason should not be empty")
	}
	for _, want := range []string{"Code", "File", "Low", "Allow"} {
		if !containsStr(r, want) {
			t.Errorf("reason %q missing %q", r, want)
		}
	}
}

func TestBuildDetails(t *testing.T) {
	action := SafetyAction{
		Type:      "command",
		Name:      "exec",
		FilePaths: []string{"a.go", "b.go"},
	}
	d := buildDetails(action, IntentCode, ScopeRepo)
	if d["intent"] != "Code" {
		t.Errorf("details.intent = %q, want Code", d["intent"])
	}
	if d["scope"] != "Repo" {
		t.Errorf("details.scope = %q, want Repo", d["scope"])
	}
	if d["name"] != "exec" {
		t.Errorf("details.name = %q, want exec", d["name"])
	}
	if d["type"] != "command" {
		t.Errorf("details.type = %q, want command", d["type"])
	}
	if d["paths"] != "a.go, b.go" {
		t.Errorf("details.paths = %q, want 'a.go, b.go'", d["paths"])
	}
}

// --- CompositeValidator Tests ---

func TestCompositeValidator_Empty(t *testing.T) {
	cv := NewCompositeValidator()
	a := cv.Validate(SafetyAction{Name: "test"})
	if a.Action != PolicyAllow {
		t.Errorf("empty composite should allow, got %v", PolicyActionName(a.Action))
	}
}

func TestCompositeValidator_SingleValidator(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	cv := NewCompositeValidator(sv)

	a := cv.Validate(SafetyAction{
		Type:      "file_read",
		Name:      "readFile",
		FilePaths: []string{"main.go"},
	})
	if a.Intent != IntentReadOnly {
		t.Errorf("intent = %v, want ReadOnly", IntentName(a.Intent))
	}
}

func TestCompositeValidator_MostRestrictive(t *testing.T) {
	// Permissive validator (disabled).
	permCfg := DefaultSafetyConfig()
	permCfg.Enabled = false
	permissive := NewSafetyValidator(permCfg)

	// Strict validator.
	strictCfg := DefaultSafetyConfig()
	strictCfg.BlockedTools = []string{"dangerExec"}
	strict := NewSafetyValidator(strictCfg)

	cv := NewCompositeValidator(permissive, strict)

	a := cv.Validate(SafetyAction{Name: "dangerExec"})
	if a.Action != PolicyBlock {
		t.Errorf("composite should use most restrictive, got %v", PolicyActionName(a.Action))
	}
}

func TestCompositeValidator_HigherRiskWins(t *testing.T) {
	// Two enabled validators with different thresholds.
	laxCfg := DefaultSafetyConfig()
	laxCfg.WarnThreshold = 0.8
	laxCfg.ConfirmThreshold = 0.9
	laxCfg.BlockThreshold = 1.0
	lax := NewSafetyValidator(laxCfg)

	strictCfg := DefaultSafetyConfig()
	strictCfg.WarnThreshold = 0.1
	strictCfg.ConfirmThreshold = 0.2
	strictCfg.BlockThreshold = 0.5
	strict := NewSafetyValidator(strictCfg)

	cv := NewCompositeValidator(lax, strict)

	a := cv.Validate(SafetyAction{
		Type: "command",
		Name: "exec",
		Raw:  "rm -rf /tmp/build",
	})

	// Strict validator should yield higher policy action.
	if a.Action < PolicyWarn {
		t.Errorf("strict validator should yield at least Warn, got %v", PolicyActionName(a.Action))
	}
}

// --- Validator Interface Compliance ---

func TestSafetyValidator_ImplementsValidator(t *testing.T) {
	var _ Validator = (*SafetyValidator)(nil)
}

func TestCompositeValidator_ImplementsValidator(t *testing.T) {
	var _ Validator = (*CompositeValidator)(nil)
}

// --- Concurrent Access ---

func TestSafetyValidator_ConcurrentValidate(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			sv.Validate(SafetyAction{
				Type: "command",
				Name: "exec",
				Raw:  "ls -la /tmp",
			})
		}(i)
	}
	wg.Wait()

	stats := sv.Stats()
	if stats.TotalChecks != n {
		t.Errorf("total checks = %d, want %d", stats.TotalChecks, n)
	}
}

func TestSafetyValidator_ConcurrentStats(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	var wg sync.WaitGroup
	wg.Add(30)
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			sv.Validate(SafetyAction{Type: "file_read", Name: "readFile"})
		}()
	}
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			_ = sv.Stats()
		}()
	}
	wg.Wait()

	stats := sv.Stats()
	if stats.TotalChecks != 20 {
		t.Errorf("total checks = %d, want 20", stats.TotalChecks)
	}
}

// --- SensitivePattern in filepath ---

func TestSafetyValidator_SensitiveFilePath(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type:      "file_read",
		Name:      "readFile",
		FilePaths: []string{".env"},
	})

	if a.Intent != IntentCredential {
		t.Errorf("intent = %v, want Credential (sensitive file path)", IntentName(a.Intent))
	}
}

func TestSafetyValidator_PemFile(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{
		Type:      "file_read",
		Name:      "readFile",
		FilePaths: []string{"server.pem"},
	})

	// .pem should match credential intent via intent pattern.
	if a.Intent != IntentCredential {
		t.Errorf("intent = %v, want Credential (.pem file)", IntentName(a.Intent))
	}
}

// --- Edge Cases ---

func TestSafetyValidator_EmptyAction(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())

	a := sv.Validate(SafetyAction{})
	// Empty action should not panic and should produce Unknown intent.
	if a.Intent != IntentUnknown {
		t.Errorf("empty action intent = %v, want Unknown", IntentName(a.Intent))
	}
}

func TestSafetyValidator_AllowedPathsNoFiles(t *testing.T) {
	cfg := DefaultSafetyConfig()
	cfg.AllowedPaths = []string{"src/*"}
	sv := NewSafetyValidator(cfg)

	// Action without file paths should not be blocked by allowlist.
	a := sv.Validate(SafetyAction{
		Type: "search",
		Name: "grepSearch",
	})
	if a.Action == PolicyBlock {
		t.Error("action without file paths should not be blocked by allowlist")
	}
}

func TestSafetyValidator_Config(t *testing.T) {
	cfg := DefaultSafetyConfig()
	cfg.BlockedTools = []string{"bad"}
	sv := NewSafetyValidator(cfg)

	got := sv.Config()
	if !got.Enabled {
		t.Error("config should be enabled")
	}
	if len(got.BlockedTools) != 1 || got.BlockedTools[0] != "bad" {
		t.Error("config.BlockedTools mismatch")
	}
}

// containsStr is a helper to check substring presence.
func containsStr(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && len(s) >= len(sub) &&
		(s == sub || len(s) > len(sub) && containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
