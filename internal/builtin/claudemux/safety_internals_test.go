package claudemux

import (
	"testing"
)

// ── classifyIntent (unexported, directly tested) ───────────────────

func TestClassifyIntent_SensitiveOverridesType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	// Even though type is "file_read" (ReadOnly), reading .env is credential access.
	action := SafetyAction{
		Type: "file_read",
		Name: "read_file",
		Raw:  "reading .env for config",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentCredential {
		t.Errorf("got %s, want Credential", IntentName(intent))
	}
}

func TestClassifyIntent_DestructiveViaPattern(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Name: "execute",
		Raw:  "rm -rf /tmp/old",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentDestructive {
		t.Errorf("got %s, want Destructive", IntentName(intent))
	}
}

func TestClassifyIntent_NetworkViaPattern(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Name: "execute",
		Raw:  "curl https://example.com/api",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentNetwork {
		t.Errorf("got %s, want Network", IntentName(intent))
	}
}

func TestClassifyIntent_ReadOnlyViaPattern(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Name: "execute",
		Raw:  "cat README.md",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentReadOnly {
		t.Errorf("got %s, want ReadOnly", IntentName(intent))
	}
}

func TestClassifyIntent_FileDeleteType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "file_delete",
		Name: "delete_file",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentDestructive {
		t.Errorf("got %s, want Destructive", IntentName(intent))
	}
}

func TestClassifyIntent_FileWriteType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "file_write",
		Name: "write_file",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentCode {
		t.Errorf("got %s, want Code", IntentName(intent))
	}
}

func TestClassifyIntent_FileCreateType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "file_create",
		Name: "create_file",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentCode {
		t.Errorf("got %s, want Code", IntentName(intent))
	}
}

func TestClassifyIntent_SearchType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "search",
		Name: "search_tool",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentReadOnly {
		t.Errorf("got %s, want ReadOnly", IntentName(intent))
	}
}

func TestClassifyIntent_ListType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "list",
		Name: "list_items",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentReadOnly {
		t.Errorf("got %s, want ReadOnly", IntentName(intent))
	}
}

func TestClassifyIntent_NetworkType(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "network",
		Name: "net_call",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentNetwork {
		t.Errorf("got %s, want Network", IntentName(intent))
	}
}

func TestClassifyIntent_ToolNameHeuristic_Write(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "unknown_type",
		Name: "write_data",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentCode {
		t.Errorf("got %s, want Code", IntentName(intent))
	}
}

func TestClassifyIntent_ToolNameHeuristic_Edit(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "unknown_type",
		Name: "edit_config",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentCode {
		t.Errorf("got %s, want Code", IntentName(intent))
	}
}

func TestClassifyIntent_ToolNameHeuristic_Create(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "unknown_type",
		Name: "create_resource",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentCode {
		t.Errorf("got %s, want Code", IntentName(intent))
	}
}

func TestClassifyIntent_ToolNameHeuristic_Read(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "unknown_type",
		Name: "read_data",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentReadOnly {
		t.Errorf("got %s, want ReadOnly", IntentName(intent))
	}
}

func TestClassifyIntent_ToolNameHeuristic_Get(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "unknown_type",
		Name: "get_info",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentReadOnly {
		t.Errorf("got %s, want ReadOnly", IntentName(intent))
	}
}

func TestClassifyIntent_ToolNameHeuristic_Delete(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "unknown_type",
		Name: "delete_item",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentDestructive {
		t.Errorf("got %s, want Destructive", IntentName(intent))
	}
}

func TestClassifyIntent_ToolNameHeuristic_Remove(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "unknown_type",
		Name: "remove_entry",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentDestructive {
		t.Errorf("got %s, want Destructive", IntentName(intent))
	}
}

func TestClassifyIntent_UnknownFallthrough(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Type: "unknown_type",
		Name: "mystery_tool",
	}
	intent := sv.classifyIntent(action)
	if intent != IntentUnknown {
		t.Errorf("got %s, want Unknown", IntentName(intent))
	}
}

// ── assessScope (unexported, directly tested) ──────────────────────

func TestAssessScope_SystemPath_Etc(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		FilePaths: []string{"/etc/passwd"},
	}
	scope := sv.assessScope(action)
	if scope != ScopeInfra {
		t.Errorf("got %s, want Infra", ScopeName(scope))
	}
}

func TestAssessScope_SystemPath_Usr(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		FilePaths: []string{"/usr/local/bin/foo"},
	}
	scope := sv.assessScope(action)
	if scope != ScopeInfra {
		t.Errorf("got %s, want Infra", ScopeName(scope))
	}
}

func TestAssessScope_SystemPath_System(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		FilePaths: []string{"/system/config"},
	}
	scope := sv.assessScope(action)
	if scope != ScopeInfra {
		t.Errorf("got %s, want Infra", ScopeName(scope))
	}
}

func TestAssessScope_ManyFiles_RepoScope(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		FilePaths: []string{"a.go", "b.go", "c.go", "d.go"},
	}
	scope := sv.assessScope(action)
	if scope != ScopeRepo {
		t.Errorf("got %s, want Repo", ScopeName(scope))
	}
}

func TestAssessScope_SingleFile_FileScope(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		FilePaths: []string{"main.go"},
	}
	scope := sv.assessScope(action)
	if scope != ScopeFile {
		t.Errorf("got %s, want File", ScopeName(scope))
	}
}

func TestAssessScope_NoFiles_Unknown(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Name: "some_tool",
	}
	scope := sv.assessScope(action)
	if scope != ScopeUnknown {
		t.Errorf("got %s, want Unknown", ScopeName(scope))
	}
}

func TestAssessScope_ThreeFiles_FileScope(t *testing.T) {
	// Three files (≤3) should return File scope, not Repo.
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		FilePaths: []string{"a.go", "b.go", "c.go"},
	}
	scope := sv.assessScope(action)
	if scope != ScopeFile {
		t.Errorf("got %s, want File (three files <= threshold)", ScopeName(scope))
	}
}

// ── enforcePolicy (unexported, directly tested) ────────────────────

func TestEnforcePolicy_BelowWarn_Allow(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	// Default warn=0.3. Risk 0.1 should be Allow.
	if p := sv.enforcePolicy(0.1); p != PolicyAllow {
		t.Errorf("got %s, want Allow", PolicyActionName(p))
	}
}

func TestEnforcePolicy_AtWarn(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	// Risk exactly at 0.3 → Warn.
	if p := sv.enforcePolicy(0.3); p != PolicyWarn {
		t.Errorf("got %s, want Warn", PolicyActionName(p))
	}
}

func TestEnforcePolicy_AtConfirm(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	// Risk exactly at 0.6 → Confirm.
	if p := sv.enforcePolicy(0.6); p != PolicyConfirm {
		t.Errorf("got %s, want Confirm", PolicyActionName(p))
	}
}

func TestEnforcePolicy_AtBlock(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	// Risk exactly at 0.9 → Block.
	if p := sv.enforcePolicy(0.9); p != PolicyBlock {
		t.Errorf("got %s, want Block", PolicyActionName(p))
	}
}

func TestEnforcePolicy_HighRisk(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	if p := sv.enforcePolicy(1.0); p != PolicyBlock {
		t.Errorf("got %s, want Block", PolicyActionName(p))
	}
}

func TestEnforcePolicy_ZeroRisk(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	if p := sv.enforcePolicy(0.0); p != PolicyAllow {
		t.Errorf("got %s, want Allow", PolicyActionName(p))
	}
}

// ── checkAllowedPaths (unexported, directly tested) ────────────────

func TestCheckAllowedPaths_NoAllowlist_NotBlocked(t *testing.T) {
	sv := NewSafetyValidator(SafetyConfig{
		Enabled:      true,
		AllowedPaths: nil, // No allowlist
	})
	blocked, _ := sv.checkAllowedPaths(SafetyAction{
		FilePaths: []string{"/any/path"},
	})
	if blocked {
		t.Error("no allowlist should not block")
	}
}

func TestCheckAllowedPaths_NoFilePaths_NotBlocked(t *testing.T) {
	sv := NewSafetyValidator(SafetyConfig{
		Enabled:      true,
		AllowedPaths: []string{"src/*"},
	})
	blocked, _ := sv.checkAllowedPaths(SafetyAction{
		FilePaths: nil,
	})
	if blocked {
		t.Error("no file paths should not block")
	}
}

func TestCheckAllowedPaths_MatchByFullPath(t *testing.T) {
	sv := NewSafetyValidator(SafetyConfig{
		Enabled:      true,
		AllowedPaths: []string{"src/*.go"},
	})
	blocked, _ := sv.checkAllowedPaths(SafetyAction{
		FilePaths: []string{"src/main.go"},
	})
	if blocked {
		t.Error("matching full path should not block")
	}
}

func TestCheckAllowedPaths_MatchByBaseName(t *testing.T) {
	sv := NewSafetyValidator(SafetyConfig{
		Enabled:      true,
		AllowedPaths: []string{"*.go"},
	})
	blocked, _ := sv.checkAllowedPaths(SafetyAction{
		FilePaths: []string{"src/main.go"},
	})
	if blocked {
		t.Error("matching base name should not block")
	}
}

func TestCheckAllowedPaths_NotInAllowlist_Blocked(t *testing.T) {
	sv := NewSafetyValidator(SafetyConfig{
		Enabled:      true,
		AllowedPaths: []string{"src/*.go"},
	})
	blocked, path := sv.checkAllowedPaths(SafetyAction{
		FilePaths: []string{"etc/config.yaml"},
	})
	if !blocked {
		t.Error("file not in allowlist should be blocked")
	}
	if path != "etc/config.yaml" {
		t.Errorf("blocked path: got %q, want %q", path, "etc/config.yaml")
	}
}

func TestCheckAllowedPaths_MultipleFiles_OneNotAllowed(t *testing.T) {
	sv := NewSafetyValidator(SafetyConfig{
		Enabled:      true,
		AllowedPaths: []string{"*.go"},
	})
	blocked, path := sv.checkAllowedPaths(SafetyAction{
		FilePaths: []string{"main.go", "secret.yaml"},
	})
	if !blocked {
		t.Error("should block when one file not allowed")
	}
	if path != "secret.yaml" {
		t.Errorf("blocked path: got %q, want %q", path, "secret.yaml")
	}
}

// ── calculateRisk (unexported, directly tested) ────────────────────

func TestCalculateRisk_ReadOnlyFile_Low(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	risk := sv.calculateRisk(IntentReadOnly, ScopeFile, SafetyAction{})
	// 0.05 * 1.0 = 0.05
	if risk < 0.04 || risk > 0.06 {
		t.Errorf("got %f, want ~0.05", risk)
	}
}

func TestCalculateRisk_DestructiveInfra_High(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	risk := sv.calculateRisk(IntentDestructive, ScopeInfra, SafetyAction{})
	// 0.7 * 1.6 = 1.12 → clamped to 1.0
	if risk != 1.0 {
		t.Errorf("got %f, want 1.0 (clamped)", risk)
	}
}

func TestCalculateRisk_SensitiveBonus(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	action := SafetyAction{
		Raw: "export API_KEY=secret123",
	}
	risk := sv.calculateRisk(IntentCode, ScopeFile, action)
	// 0.25 * 1.0 + 0.2 = 0.45
	if risk < 0.44 || risk > 0.46 {
		t.Errorf("got %f, want ~0.45", risk)
	}
}

func TestCalculateRisk_UnknownScopeMultiplier(t *testing.T) {
	sv := NewSafetyValidator(DefaultSafetyConfig())
	risk := sv.calculateRisk(IntentCode, ScopeUnknown, SafetyAction{})
	// 0.25 * 1.1 = 0.275
	if risk < 0.27 || risk > 0.28 {
		t.Errorf("got %f, want ~0.275", risk)
	}
}
