package command

import "github.com/joeycumines/one-shot-man/internal/triage"

// Re-export triage types for command package compatibility.
type TriageKind = triage.TriageKind
type TriageResult = triage.TriageResult

const (
	Trivial          = triage.Trivial
	SemanticReview   = triage.SemanticReview
	HighRiskSecurity = triage.HighRiskSecurity
)

// TriageDiff classifies each file diff in a unified diff.
func TriageDiff(diff string) []TriageResult { return triage.TriageDiff(diff) }

// TriageSummary returns counts per kind.
func TriageSummary(results []TriageResult) map[TriageKind]int {
	return triage.TriageSummary(results)
}
