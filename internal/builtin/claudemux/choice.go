package claudemux

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
)

// Candidate represents a possible choice in a decision.
type Candidate struct {
	ID          string            // Unique identifier for this candidate
	Name        string            // Human-readable name
	Description string            // Detailed description
	Attributes  map[string]string // Key-value attributes for criteria evaluation
}

// Criterion defines a dimension for evaluating candidates.
type Criterion struct {
	Name        string  // Criterion name (e.g., "complexity", "risk", "maintainability")
	Weight      float64 // Relative weight (0.0 to 1.0). Weights are normalized before scoring.
	Description string  // What this criterion measures
}

// CandidateScore holds the evaluation result for a single candidate.
type CandidateScore struct {
	CandidateID string
	Name        string
	TotalScore  float64            // Weighted total score (0.0 to 1.0)
	Scores      map[string]float64 // Per-criterion raw scores (0.0 to 1.0)
	Rank        int                // 1-based rank (1 = best)
	Justification string           // Human-readable justification
}

// ChoiceResult holds the output of a choice analysis.
type ChoiceResult struct {
	RecommendedID string            // ID of the top-ranked candidate
	Rankings      []CandidateScore  // All candidates sorted by score desc
	Justification string            // Overall justification for the recommendation
	NeedsConfirm  bool              // Whether user confirmation is required
}

// ScoreFunc evaluates a candidate on a specific criterion and returns
// a score between 0.0 (worst) and 1.0 (best).
type ScoreFunc func(candidate Candidate, criterion Criterion) float64

// ChoiceConfig configures the choice resolver.
type ChoiceConfig struct {
	// DefaultCriteria are the standard criteria applied when none are specified.
	DefaultCriteria []Criterion

	// ConfirmThreshold — recommendations with TotalScore below this require confirmation.
	// Default: 0.5
	ConfirmThreshold float64

	// MinCandidates — minimum candidates required for analysis. Default: 2.
	MinCandidates int
}

// DefaultChoiceConfig returns production-ready defaults.
func DefaultChoiceConfig() ChoiceConfig {
	return ChoiceConfig{
		DefaultCriteria: []Criterion{
			{Name: "complexity", Weight: 0.3, Description: "Implementation complexity (lower is better)"},
			{Name: "risk", Weight: 0.3, Description: "Risk of failure or regression (lower is better)"},
			{Name: "maintainability", Weight: 0.2, Description: "Long-term maintainability (higher is better)"},
			{Name: "performance", Weight: 0.2, Description: "Runtime performance impact (higher is better)"},
		},
		ConfirmThreshold: 0.5,
		MinCandidates:    2,
	}
}

// ChoiceResolver evaluates multiple candidates against weighted criteria
// and produces a ranked recommendation.
// Thread-safe: all methods may be called from any goroutine.
type ChoiceResolver struct {
	config ChoiceConfig

	mu    sync.RWMutex
	stats ChoiceStats
}

// ChoiceStats tracks resolver usage.
type ChoiceStats struct {
	TotalAnalyses   int64
	TotalCandidates int64
	ConfirmCount    int64 // Number of times confirmation was required
}

// NewChoiceResolver creates a resolver with the given configuration.
func NewChoiceResolver(cfg ChoiceConfig) *ChoiceResolver {
	if cfg.MinCandidates < 1 {
		cfg.MinCandidates = 1
	}
	if cfg.ConfirmThreshold <= 0 {
		cfg.ConfirmThreshold = 0.5
	}
	return &ChoiceResolver{
		config: cfg,
	}
}

// Analyze evaluates candidates against criteria using the provided scoring function.
// If criteria is nil, uses config.DefaultCriteria.
// If scoreFn is nil, uses attribute-based default scoring.
func (cr *ChoiceResolver) Analyze(candidates []Candidate, criteria []Criterion, scoreFn ScoreFunc) (ChoiceResult, error) {
	if len(candidates) < cr.config.MinCandidates {
		return ChoiceResult{}, fmt.Errorf("claudemux: need at least %d candidates, got %d",
			cr.config.MinCandidates, len(candidates))
	}

	if len(criteria) == 0 {
		criteria = cr.config.DefaultCriteria
	}

	if scoreFn == nil {
		scoreFn = attributeScoreFunc
	}

	// Normalize weights.
	weights := normalizeCriteriaWeights(criteria)

	// Score each candidate.
	rankings := make([]CandidateScore, len(candidates))
	for i, cand := range candidates {
		scores := make(map[string]float64, len(criteria))
		var totalScore float64

		for j, crit := range criteria {
			raw := scoreFn(cand, crit)
			raw = clampScore(raw)
			scores[crit.Name] = raw
			totalScore += raw * weights[j]
		}

		rankings[i] = CandidateScore{
			CandidateID:   cand.ID,
			Name:          cand.Name,
			TotalScore:    totalScore,
			Scores:        scores,
			Justification: buildCandidateJustification(cand, scores, criteria),
		}
	}

	// Sort by total score descending.
	sort.Slice(rankings, func(i, j int) bool {
		return rankings[i].TotalScore > rankings[j].TotalScore
	})

	// Assign ranks (1-based).
	for i := range rankings {
		rankings[i].Rank = i + 1
	}

	needsConfirm := len(rankings) > 0 && rankings[0].TotalScore < cr.config.ConfirmThreshold
	result := ChoiceResult{
		Rankings:      rankings,
		NeedsConfirm:  needsConfirm,
	}

	if len(rankings) > 0 {
		result.RecommendedID = rankings[0].CandidateID
		result.Justification = buildOverallJustification(rankings)
	}

	// Update stats.
	cr.mu.Lock()
	cr.stats.TotalAnalyses++
	cr.stats.TotalCandidates += int64(len(candidates))
	if needsConfirm {
		cr.stats.ConfirmCount++
	}
	cr.mu.Unlock()

	return result, nil
}

// Stats returns resolver usage statistics.
func (cr *ChoiceResolver) Stats() ChoiceStats {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return cr.stats
}

// Config returns the current configuration.
func (cr *ChoiceResolver) Config() ChoiceConfig {
	return cr.config
}

// --- Attribute-based default scoring ---

// attributeScoreFunc is the default scoring function. It looks for an attribute
// with the same name as the criterion and parses it as a float, or returns 0.5
// if not found.
func attributeScoreFunc(candidate Candidate, criterion Criterion) float64 {
	if candidate.Attributes == nil {
		return 0.5
	}
	val, ok := candidate.Attributes[criterion.Name]
	if !ok {
		return 0.5
	}
	// Parse as float.
	var f float64
	_, err := fmt.Sscanf(val, "%f", &f)
	if err != nil {
		return 0.5
	}
	return clampScore(f)
}

// --- Helpers ---

// normalizeCriteriaWeights normalizes criterion weights so they sum to 1.0.
func normalizeCriteriaWeights(criteria []Criterion) []float64 {
	weights := make([]float64, len(criteria))
	var sum float64
	for i, c := range criteria {
		w := c.Weight
		if w < 0 {
			w = 0
		}
		weights[i] = w
		sum += w
	}
	if sum == 0 {
		// Equal weighting fallback.
		for i := range weights {
			weights[i] = 1.0 / float64(len(weights))
		}
	} else {
		for i := range weights {
			weights[i] /= sum
		}
	}
	return weights
}

// clampScore clamps a score to [0.0, 1.0].
func clampScore(s float64) float64 {
	if math.IsNaN(s) || math.IsInf(s, 0) {
		return 0.5
	}
	if s < 0 {
		return 0
	}
	if s > 1 {
		return 1
	}
	return s
}

// buildCandidateJustification creates a per-candidate justification string.
func buildCandidateJustification(cand Candidate, scores map[string]float64, criteria []Criterion) string {
	var b strings.Builder
	b.WriteString(cand.Name)
	b.WriteString(": ")
	first := true
	for _, crit := range criteria {
		if !first {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%s=%.2f", crit.Name, scores[crit.Name]))
		first = false
	}
	return b.String()
}

// buildOverallJustification creates an overall recommendation justification.
func buildOverallJustification(rankings []CandidateScore) string {
	if len(rankings) == 0 {
		return "no candidates"
	}
	top := rankings[0]
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Recommended: %s (score: %.3f)", top.Name, top.TotalScore))
	if len(rankings) > 1 {
		runner := rankings[1]
		diff := top.TotalScore - runner.TotalScore
		b.WriteString(fmt.Sprintf(", margin: %.3f over %s", diff, runner.Name))
	}
	return b.String()
}
