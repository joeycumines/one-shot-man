package claudemux

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// Intent classifies the purpose of an action.
type Intent int

const (
	IntentUnknown     Intent = iota // Unable to classify
	IntentReadOnly                  // Read-only operations (view, list, search)
	IntentCode                      // Code modifications (edit, create, refactor)
	IntentDestructive               // Destructive operations (delete, overwrite, format disk)
	IntentNetwork                   // Network access (fetch, API calls, download)
	IntentCredential                // Credential/secret access (keys, tokens, passwords)
)

// IntentName returns a human-readable name for an Intent.
func IntentName(i Intent) string {
	switch i {
	case IntentUnknown:
		return "Unknown"
	case IntentReadOnly:
		return "ReadOnly"
	case IntentCode:
		return "Code"
	case IntentDestructive:
		return "Destructive"
	case IntentNetwork:
		return "Network"
	case IntentCredential:
		return "Credential"
	default:
		return fmt.Sprintf("Unknown(%d)", int(i))
	}
}

// Scope assesses the blast radius of an action.
type Scope int

const (
	ScopeUnknown Scope = iota // Unable to determine
	ScopeFile                 // Single file operation
	ScopeRepo                 // Repository-wide operation
	ScopeInfra                // Infrastructure-level (system commands, env, etc.)
)

// ScopeName returns a human-readable name for a Scope.
func ScopeName(s Scope) string {
	switch s {
	case ScopeUnknown:
		return "Unknown"
	case ScopeFile:
		return "File"
	case ScopeRepo:
		return "Repo"
	case ScopeInfra:
		return "Infra"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// RiskLevel categorizes assessed risk.
type RiskLevel int

const (
	RiskNone     RiskLevel = iota // No risk
	RiskLow                       // Low risk — proceed freely
	RiskMedium                    // Medium risk — log/warn
	RiskHigh                      // High risk — require confirmation
	RiskCritical                  // Critical risk — block
)

// RiskLevelName returns a human-readable name for a RiskLevel.
func RiskLevelName(l RiskLevel) string {
	switch l {
	case RiskNone:
		return "None"
	case RiskLow:
		return "Low"
	case RiskMedium:
		return "Medium"
	case RiskHigh:
		return "High"
	case RiskCritical:
		return "Critical"
	default:
		return fmt.Sprintf("Unknown(%d)", int(l))
	}
}

// PolicyAction is the enforcement decision for an action.
type PolicyAction int

const (
	PolicyAllow   PolicyAction = iota // Proceed freely
	PolicyWarn                        // Log warning but allow
	PolicyConfirm                     // Require user confirmation
	PolicyBlock                       // Reject the action
)

// PolicyActionName returns a human-readable name for a PolicyAction.
func PolicyActionName(a PolicyAction) string {
	switch a {
	case PolicyAllow:
		return "Allow"
	case PolicyWarn:
		return "Warn"
	case PolicyConfirm:
		return "Confirm"
	case PolicyBlock:
		return "Block"
	default:
		return fmt.Sprintf("Unknown(%d)", int(a))
	}
}

// SafetyAction is the input to a safety validator — the action being assessed.
type SafetyAction struct {
	Type      string            // Action type: "tool_call", "command", "file_write", "file_delete"
	Name      string            // Tool or command name
	Args      map[string]string // Arguments/parameters
	FilePaths []string          // Relevant file paths
	Raw       string            // Raw text for pattern/AI classification
}

// SafetyAssessment is the result of evaluating an action.
type SafetyAssessment struct {
	Intent    Intent
	Scope     Scope
	RiskScore float64 // 0.0 (safe) to 1.0 (critical)
	RiskLevel RiskLevel
	Action    PolicyAction
	Reason    string
	Details   map[string]string
}

// Validator evaluates a safety action and returns an assessment.
// Implementations must be safe for concurrent use.
type Validator interface {
	Validate(action SafetyAction) SafetyAssessment
}

// --- Rule-Based Classifier ---

// intentPattern maps a regex to an intent classification.
type intentPattern struct {
	pattern *regexp.Regexp
	intent  Intent
}

// scopePattern maps a regex to a scope classification.
type scopePattern struct {
	pattern *regexp.Regexp
	scope   Scope
}

// SafetyConfig holds configuration for the rule-based safety validator.
type SafetyConfig struct {
	Enabled bool // Master switch — if false, all actions are allowed

	// DefaultAction when no rules match. Defaults to PolicyAllow.
	DefaultAction PolicyAction

	// Risk thresholds for policy actions.
	// Actions with risk ≥ threshold get that policy.
	WarnThreshold    float64 // Default: 0.3
	ConfirmThreshold float64 // Default: 0.6
	BlockThreshold   float64 // Default: 0.9

	// BlockedTools are tool names that are always blocked.
	BlockedTools []string

	// BlockedPaths are path patterns (glob) that are always blocked.
	BlockedPaths []string

	// AllowedPaths are path patterns (glob) that bypass safety checks.
	// If non-empty, only paths matching these patterns are allowed.
	AllowedPaths []string

	// SensitivePatterns are regexps matching sensitive content in args/raw text.
	SensitivePatterns []string
}

// DefaultSafetyConfig returns a production-ready safety configuration.
func DefaultSafetyConfig() SafetyConfig {
	return SafetyConfig{
		Enabled:          true,
		DefaultAction:    PolicyAllow,
		WarnThreshold:    0.3,
		ConfirmThreshold: 0.6,
		BlockThreshold:   0.9,
		BlockedTools:     nil,
		BlockedPaths:     nil,
		AllowedPaths:     nil,
		SensitivePatterns: []string{
			`(?i)(api[_-]?key|secret|token|password|credential|private[_-]?key)`,
		},
	}
}

// SafetyValidator is a rule-based implementation of Validator.
// Thread-safe: all methods may be called from any goroutine.
type SafetyValidator struct {
	config SafetyConfig

	mu    sync.RWMutex
	stats SafetyStats

	// Precompiled patterns.
	intentPatterns    []intentPattern
	scopePatterns     []scopePattern
	sensitivePatterns []*regexp.Regexp
	blockedTools      map[string]bool
}

// SafetyStats tracks safety validation statistics.
type SafetyStats struct {
	TotalChecks  int64
	AllowCount   int64
	WarnCount    int64
	ConfirmCount int64
	BlockCount   int64
	IntentCounts map[string]int64 // IntentName -> count
	ScopeCounts  map[string]int64 // ScopeName -> count
}

// NewSafetyValidator creates a safety validator with the given configuration.
func NewSafetyValidator(cfg SafetyConfig) *SafetyValidator {
	sv := &SafetyValidator{
		config:       cfg,
		blockedTools: make(map[string]bool, len(cfg.BlockedTools)),
		stats: SafetyStats{
			IntentCounts: make(map[string]int64),
			ScopeCounts:  make(map[string]int64),
		},
	}

	// Build blocked tools set.
	for _, t := range cfg.BlockedTools {
		sv.blockedTools[t] = true
	}

	// Compile sensitive patterns.
	for _, p := range cfg.SensitivePatterns {
		re, err := regexp.Compile(p)
		if err == nil {
			sv.sensitivePatterns = append(sv.sensitivePatterns, re)
		}
	}

	// Register built-in intent patterns.
	sv.registerIntentPatterns()

	// Register built-in scope patterns.
	sv.registerScopePatterns()

	return sv
}

// registerIntentPatterns sets up the default intent classification rules.
func (sv *SafetyValidator) registerIntentPatterns() {
	// Destructive: rm, delete, drop, truncate, format, overwrite
	sv.intentPatterns = append(sv.intentPatterns, intentPattern{
		pattern: regexp.MustCompile(`(?i)\b(rm\s+-rf|rm\s|rmdir|del\s|delete|drop\s|truncate|format\s|shred|wipe)\b`),
		intent:  IntentDestructive,
	})

	// Credential: access to secrets, keys, tokens
	sv.intentPatterns = append(sv.intentPatterns, intentPattern{
		pattern: regexp.MustCompile(`(?i)(\.env|\.pem|\.key|id_rsa|id_ed25519|\.ssh/|credentials|secrets?\.ya?ml|vault|keychain|keyring)`),
		intent:  IntentCredential,
	})

	// Network: curl, wget, fetch, http, socket, net
	sv.intentPatterns = append(sv.intentPatterns, intentPattern{
		pattern: regexp.MustCompile(`(?i)\b(curl|wget|fetch|http[s]?://|socket|nc\s|ncat|nmap|ssh\s|scp\s|rsync\s)\b`),
		intent:  IntentNetwork,
	})

	// ReadOnly: cat, less, head, tail, find, grep, ls, dir
	sv.intentPatterns = append(sv.intentPatterns, intentPattern{
		pattern: regexp.MustCompile(`(?i)\b(cat\s|less\s|head\s|tail\s|find\s|grep\s|ls\s|dir\s|type\s|more\s|wc\s)\b`),
		intent:  IntentReadOnly,
	})

	// Code: edit, write, create, modify, patch, sed, awk
	sv.intentPatterns = append(sv.intentPatterns, intentPattern{
		pattern: regexp.MustCompile(`(?i)\b(edit|write|create|modify|patch|sed\s|awk\s|append|insert|replace)\b`),
		intent:  IntentCode,
	})
}

// registerScopePatterns sets up the default scope classification rules.
func (sv *SafetyValidator) registerScopePatterns() {
	// Infra: system directories, env manipulation, sudo, service control
	sv.scopePatterns = append(sv.scopePatterns, scopePattern{
		pattern: regexp.MustCompile(`(?i)(^/etc/|^/usr/|^/system|^C:\\Windows|sudo\s|systemctl|service\s|launchctl|registry|env\s|export\s|setx\s)`),
		scope:   ScopeInfra,
	})

	// Repo: git operations, multi-file globs, directory-wide ops
	sv.scopePatterns = append(sv.scopePatterns, scopePattern{
		pattern: regexp.MustCompile(`(?i)(\bgit\s+(push|reset|rebase|force)|\.git/|\*\*\/|\.\.\.|find\s+\.\s)`),
		scope:   ScopeRepo,
	})
}

// Validate classifies an action and returns a safety assessment.
func (sv *SafetyValidator) Validate(action SafetyAction) SafetyAssessment {
	// When disabled, allow everything with zero risk.
	if !sv.config.Enabled {
		a := SafetyAssessment{
			Intent:    IntentUnknown,
			Scope:     ScopeUnknown,
			RiskScore: 0,
			RiskLevel: RiskNone,
			Action:    PolicyAllow,
			Reason:    "safety validation disabled",
		}
		sv.recordStats(a)
		return a
	}

	// Step 1: Check blocked tools.
	if sv.blockedTools[action.Name] {
		a := SafetyAssessment{
			Intent:    IntentDestructive,
			Scope:     ScopeInfra,
			RiskScore: 1.0,
			RiskLevel: RiskCritical,
			Action:    PolicyBlock,
			Reason:    fmt.Sprintf("tool %q is blocked by policy", action.Name),
			Details:   map[string]string{"tool": action.Name},
		}
		sv.recordStats(a)
		return a
	}

	// Step 2: Check blocked paths.
	if blocked, path := sv.matchBlockedPath(action); blocked {
		a := SafetyAssessment{
			Intent:    IntentDestructive,
			Scope:     ScopeInfra,
			RiskScore: 1.0,
			RiskLevel: RiskCritical,
			Action:    PolicyBlock,
			Reason:    fmt.Sprintf("path %q is blocked by policy", path),
			Details:   map[string]string{"path": path},
		}
		sv.recordStats(a)
		return a
	}

	// Step 3: Check allowed paths (if configured — paths not in allowlist are blocked).
	if blocked, path := sv.checkAllowedPaths(action); blocked {
		a := SafetyAssessment{
			Intent:    IntentCode,
			Scope:     ScopeFile,
			RiskScore: 0.8,
			RiskLevel: RiskHigh,
			Action:    PolicyBlock,
			Reason:    fmt.Sprintf("path %q is not in allowlist", path),
			Details:   map[string]string{"path": path},
		}
		sv.recordStats(a)
		return a
	}

	// Step 4: Classify intent.
	intent := sv.classifyIntent(action)

	// Step 5: Assess scope.
	scope := sv.assessScope(action)

	// Step 6: Calculate risk score.
	riskScore := sv.calculateRisk(intent, scope, action)

	// Step 7: Determine risk level.
	riskLevel := riskLevelFromScore(riskScore)

	// Step 8: Determine policy action.
	policyAction := sv.enforcePolicy(riskScore)

	// Step 9: Build assessment.
	reason := buildReason(intent, scope, riskLevel, policyAction)
	details := buildDetails(action, intent, scope)

	a := SafetyAssessment{
		Intent:    intent,
		Scope:     scope,
		RiskScore: riskScore,
		RiskLevel: riskLevel,
		Action:    policyAction,
		Reason:    reason,
		Details:   details,
	}
	sv.recordStats(a)
	return a
}

// Stats returns the current validation statistics.
func (sv *SafetyValidator) Stats() SafetyStats {
	sv.mu.RLock()
	defer sv.mu.RUnlock()

	intentCounts := make(map[string]int64, len(sv.stats.IntentCounts))
	maps.Copy(intentCounts, sv.stats.IntentCounts)
	scopeCounts := make(map[string]int64, len(sv.stats.ScopeCounts))
	maps.Copy(scopeCounts, sv.stats.ScopeCounts)

	return SafetyStats{
		TotalChecks:  sv.stats.TotalChecks,
		AllowCount:   sv.stats.AllowCount,
		WarnCount:    sv.stats.WarnCount,
		ConfirmCount: sv.stats.ConfirmCount,
		BlockCount:   sv.stats.BlockCount,
		IntentCounts: intentCounts,
		ScopeCounts:  scopeCounts,
	}
}

// Config returns the current configuration.
func (sv *SafetyValidator) Config() SafetyConfig {
	return sv.config
}

// --- Composable Validators ---

// CompositeValidator chains multiple validators, returning the most
// restrictive assessment.
type CompositeValidator struct {
	validators []Validator
}

// NewCompositeValidator creates a validator that runs all child validators
// and returns the most restrictive result.
func NewCompositeValidator(validators ...Validator) *CompositeValidator {
	return &CompositeValidator{validators: validators}
}

// Validate runs all child validators and returns the most restrictive assessment.
func (cv *CompositeValidator) Validate(action SafetyAction) SafetyAssessment {
	if len(cv.validators) == 0 {
		return SafetyAssessment{
			Intent:    IntentUnknown,
			Scope:     ScopeUnknown,
			RiskScore: 0,
			RiskLevel: RiskNone,
			Action:    PolicyAllow,
			Reason:    "no validators configured",
		}
	}

	var most SafetyAssessment
	for i, v := range cv.validators {
		a := v.Validate(action)
		if i == 0 || a.Action > most.Action || (a.Action == most.Action && a.RiskScore > most.RiskScore) {
			most = a
		}
	}
	return most
}

// --- Internal Classification Logic ---

// classifyIntent determines the primary intent of an action.
func (sv *SafetyValidator) classifyIntent(action SafetyAction) Intent {
	// Build search text from all fields.
	searchText := buildSearchText(action)

	// Check sensitive patterns FIRST — credential access overrides type-based classification.
	for _, re := range sv.sensitivePatterns {
		if re.MatchString(searchText) {
			return IntentCredential
		}
	}

	// Check intent patterns (destructive, credential, network, etc.) before type fallback.
	for _, ip := range sv.intentPatterns {
		if ip.pattern.MatchString(searchText) {
			return ip.intent
		}
	}

	// Check action type (explicit classification).
	switch action.Type {
	case "file_delete":
		return IntentDestructive
	case "file_write", "file_create":
		return IntentCode
	case "file_read", "search", "list":
		return IntentReadOnly
	case "network", "fetch", "api_call":
		return IntentNetwork
	}

	// Default based on tool name heuristics.
	switch {
	case strings.Contains(strings.ToLower(action.Name), "write") ||
		strings.Contains(strings.ToLower(action.Name), "edit") ||
		strings.Contains(strings.ToLower(action.Name), "create"):
		return IntentCode
	case strings.Contains(strings.ToLower(action.Name), "read") ||
		strings.Contains(strings.ToLower(action.Name), "get") ||
		strings.Contains(strings.ToLower(action.Name), "list"):
		return IntentReadOnly
	case strings.Contains(strings.ToLower(action.Name), "delete") ||
		strings.Contains(strings.ToLower(action.Name), "remove"):
		return IntentDestructive
	}

	return IntentUnknown
}

// assessScope determines the blast radius of an action.
func (sv *SafetyValidator) assessScope(action SafetyAction) Scope {
	searchText := buildSearchText(action)

	// Check scope patterns.
	for _, sp := range sv.scopePatterns {
		if sp.pattern.MatchString(searchText) {
			return sp.scope
		}
	}

	// Heuristic: multiple file paths → repo scope.
	if len(action.FilePaths) > 3 {
		return ScopeRepo
	}

	// Heuristic: path depth and system directories.
	// Normalize to forward slashes for consistent cross-platform matching.
	for _, p := range action.FilePaths {
		clean := filepath.ToSlash(filepath.Clean(p))
		if strings.HasPrefix(clean, "/etc") || strings.HasPrefix(clean, "/usr") ||
			strings.HasPrefix(clean, "/system") || strings.HasPrefix(strings.ToUpper(clean), "C:/WINDOWS") {
			return ScopeInfra
		}
	}

	// Single file or no files → file scope.
	if len(action.FilePaths) > 0 {
		return ScopeFile
	}

	return ScopeUnknown
}

// calculateRisk produces a 0.0–1.0 risk score from intent, scope, and action.
func (sv *SafetyValidator) calculateRisk(intent Intent, scope Scope, action SafetyAction) float64 {
	// Base risk from intent.
	var intentRisk float64
	switch intent {
	case IntentReadOnly:
		intentRisk = 0.05
	case IntentCode:
		intentRisk = 0.25
	case IntentNetwork:
		intentRisk = 0.4
	case IntentDestructive:
		intentRisk = 0.7
	case IntentCredential:
		intentRisk = 0.8
	case IntentUnknown:
		intentRisk = 0.15
	}

	// Scope multiplier.
	var scopeMult float64
	switch scope {
	case ScopeFile:
		scopeMult = 1.0
	case ScopeRepo:
		scopeMult = 1.3
	case ScopeInfra:
		scopeMult = 1.6
	case ScopeUnknown:
		scopeMult = 1.1
	}

	risk := intentRisk * scopeMult

	// Check for sensitive content in args/raw (bonus risk).
	searchText := buildSearchText(action)
	for _, re := range sv.sensitivePatterns {
		if re.MatchString(searchText) {
			risk += 0.2
			break
		}
	}

	// Clamp to [0, 1].
	if risk > 1.0 {
		risk = 1.0
	}
	if risk < 0.0 {
		risk = 0.0
	}

	return risk
}

// enforcePolicy determines the policy action based on risk score and thresholds.
func (sv *SafetyValidator) enforcePolicy(riskScore float64) PolicyAction {
	switch {
	case riskScore >= sv.config.BlockThreshold:
		return PolicyBlock
	case riskScore >= sv.config.ConfirmThreshold:
		return PolicyConfirm
	case riskScore >= sv.config.WarnThreshold:
		return PolicyWarn
	default:
		return PolicyAllow
	}
}

// matchBlockedPath checks if any file path matches a blocked pattern.
func (sv *SafetyValidator) matchBlockedPath(action SafetyAction) (bool, string) {
	for _, fp := range action.FilePaths {
		for _, blocked := range sv.config.BlockedPaths {
			matched, err := filepath.Match(blocked, fp)
			if err == nil && matched {
				return true, fp
			}
			// Also try matching the base name.
			matched, err = filepath.Match(blocked, filepath.Base(fp))
			if err == nil && matched {
				return true, fp
			}
		}
	}
	return false, ""
}

// checkAllowedPaths checks if file paths are within the allowlist (if configured).
func (sv *SafetyValidator) checkAllowedPaths(action SafetyAction) (bool, string) {
	if len(sv.config.AllowedPaths) == 0 {
		return false, "" // No allowlist = everything allowed.
	}
	if len(action.FilePaths) == 0 {
		return false, "" // No paths to check.
	}
	for _, fp := range action.FilePaths {
		allowed := false
		for _, pattern := range sv.config.AllowedPaths {
			matched, err := filepath.Match(pattern, fp)
			if err == nil && matched {
				allowed = true
				break
			}
			matched, err = filepath.Match(pattern, filepath.Base(fp))
			if err == nil && matched {
				allowed = true
				break
			}
		}
		if !allowed {
			return true, fp
		}
	}
	return false, ""
}

// recordStats updates validation statistics.
func (sv *SafetyValidator) recordStats(a SafetyAssessment) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.stats.TotalChecks++
	sv.stats.IntentCounts[IntentName(a.Intent)]++
	sv.stats.ScopeCounts[ScopeName(a.Scope)]++
	switch a.Action {
	case PolicyAllow:
		sv.stats.AllowCount++
	case PolicyWarn:
		sv.stats.WarnCount++
	case PolicyConfirm:
		sv.stats.ConfirmCount++
	case PolicyBlock:
		sv.stats.BlockCount++
	}
}

// --- Helpers ---

// buildSearchText concatenates all action fields for pattern matching.
func buildSearchText(action SafetyAction) string {
	var b strings.Builder
	b.WriteString(action.Type)
	b.WriteByte(' ')
	b.WriteString(action.Name)
	b.WriteByte(' ')
	b.WriteString(action.Raw)
	for _, p := range action.FilePaths {
		b.WriteByte(' ')
		b.WriteString(p)
	}
	for k, v := range action.Args {
		b.WriteByte(' ')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(v)
	}
	return b.String()
}

// riskLevelFromScore converts a float risk score to a RiskLevel.
func riskLevelFromScore(score float64) RiskLevel {
	switch {
	case score >= 0.8:
		return RiskCritical
	case score >= 0.6:
		return RiskHigh
	case score >= 0.3:
		return RiskMedium
	case score > 0.05:
		return RiskLow
	default:
		return RiskNone
	}
}

// buildReason constructs a human-readable reason string.
func buildReason(intent Intent, scope Scope, risk RiskLevel, action PolicyAction) string {
	return fmt.Sprintf("%s intent at %s scope → %s risk → %s",
		IntentName(intent), ScopeName(scope), RiskLevelName(risk), PolicyActionName(action))
}

// buildDetails constructs structured detail metadata.
func buildDetails(action SafetyAction, intent Intent, scope Scope) map[string]string {
	d := map[string]string{
		"intent": IntentName(intent),
		"scope":  ScopeName(scope),
	}
	if action.Name != "" {
		d["name"] = action.Name
	}
	if action.Type != "" {
		d["type"] = action.Type
	}
	if len(action.FilePaths) > 0 {
		d["paths"] = strings.Join(action.FilePaths, ", ")
	}
	return d
}
