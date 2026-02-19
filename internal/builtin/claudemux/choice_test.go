package claudemux

import (
	"fmt"
	"math"
	"sync"
	"testing"
)

func TestDefaultChoiceConfig(t *testing.T) {
	cfg := DefaultChoiceConfig()
	if len(cfg.DefaultCriteria) == 0 {
		t.Error("default config should have criteria")
	}
	if cfg.ConfirmThreshold != 0.5 {
		t.Errorf("confirm threshold = %v, want 0.5", cfg.ConfirmThreshold)
	}
	if cfg.MinCandidates != 2 {
		t.Errorf("min candidates = %d, want 2", cfg.MinCandidates)
	}
	// Check weights sum ~1.0.
	var sum float64
	for _, c := range cfg.DefaultCriteria {
		sum += c.Weight
	}
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("default criteria weights sum = %v, want ~1.0", sum)
	}
}

func TestNewChoiceResolver(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())
	if cr == nil {
		t.Fatal("NewChoiceResolver returned nil")
	}
}

func TestNewChoiceResolver_ClampMinCandidates(t *testing.T) {
	cfg := DefaultChoiceConfig()
	cfg.MinCandidates = 0
	cr := NewChoiceResolver(cfg)
	if cr.config.MinCandidates != 1 {
		t.Errorf("minCandidates = %d, want 1 (clamped)", cr.config.MinCandidates)
	}
}

func TestNewChoiceResolver_ClampConfirmThreshold(t *testing.T) {
	cfg := DefaultChoiceConfig()
	cfg.ConfirmThreshold = -1
	cr := NewChoiceResolver(cfg)
	if cr.config.ConfirmThreshold != 0.5 {
		t.Errorf("confirmThreshold = %v, want 0.5 (default)", cr.config.ConfirmThreshold)
	}
}

func TestChoiceResolver_TooFewCandidates(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	_, err := cr.Analyze([]Candidate{{ID: "a", Name: "A"}}, nil, nil)
	if err == nil {
		t.Error("expected error for too few candidates")
	}
}

func TestChoiceResolver_BasicAnalysis(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	candidates := []Candidate{
		{ID: "a", Name: "Option A", Attributes: map[string]string{
			"complexity": "0.8", "risk": "0.7", "maintainability": "0.9", "performance": "0.8",
		}},
		{ID: "b", Name: "Option B", Attributes: map[string]string{
			"complexity": "0.3", "risk": "0.4", "maintainability": "0.5", "performance": "0.5",
		}},
	}

	result, err := cr.Analyze(candidates, nil, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if result.RecommendedID != "a" {
		t.Errorf("recommended = %q, want %q", result.RecommendedID, "a")
	}
	if len(result.Rankings) != 2 {
		t.Fatalf("rankings = %d, want 2", len(result.Rankings))
	}
	if result.Rankings[0].Rank != 1 {
		t.Errorf("rank[0] = %d, want 1", result.Rankings[0].Rank)
	}
	if result.Rankings[1].Rank != 2 {
		t.Errorf("rank[1] = %d, want 2", result.Rankings[1].Rank)
	}
	if result.Rankings[0].TotalScore <= result.Rankings[1].TotalScore {
		t.Error("first rank should have higher score than second")
	}
}

func TestChoiceResolver_CustomCriteria(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	criteria := []Criterion{
		{Name: "speed", Weight: 1.0, Description: "How fast"},
	}

	candidates := []Candidate{
		{ID: "fast", Name: "Fast", Attributes: map[string]string{"speed": "0.9"}},
		{ID: "slow", Name: "Slow", Attributes: map[string]string{"speed": "0.2"}},
	}

	result, err := cr.Analyze(candidates, criteria, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if result.RecommendedID != "fast" {
		t.Errorf("recommended = %q, want %q", result.RecommendedID, "fast")
	}
}

func TestChoiceResolver_CustomScoreFunc(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	criteria := []Criterion{
		{Name: "length", Weight: 1.0, Description: "Name length score"},
	}
	scoreFn := func(cand Candidate, crit Criterion) float64 {
		// Score based on name length (more is better, max 10).
		return float64(len(cand.Name)) / 10.0
	}

	candidates := []Candidate{
		{ID: "short", Name: "AB"},
		{ID: "long", Name: "ABCDEFGHIJ"},
	}

	result, err := cr.Analyze(candidates, criteria, scoreFn)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if result.RecommendedID != "long" {
		t.Errorf("recommended = %q, want %q", result.RecommendedID, "long")
	}
}

func TestChoiceResolver_NeedsConfirm(t *testing.T) {
	cfg := DefaultChoiceConfig()
	cfg.ConfirmThreshold = 0.8
	cr := NewChoiceResolver(cfg)

	candidates := []Candidate{
		{ID: "a", Name: "A", Attributes: map[string]string{
			"complexity": "0.5", "risk": "0.5", "maintainability": "0.5", "performance": "0.5",
		}},
		{ID: "b", Name: "B", Attributes: map[string]string{
			"complexity": "0.4", "risk": "0.4", "maintainability": "0.4", "performance": "0.4",
		}},
	}

	result, err := cr.Analyze(candidates, nil, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if !result.NeedsConfirm {
		t.Error("low scores should need confirmation with high threshold")
	}
}

func TestChoiceResolver_NoConfirmNeeded(t *testing.T) {
	cfg := DefaultChoiceConfig()
	cfg.ConfirmThreshold = 0.3
	cr := NewChoiceResolver(cfg)

	candidates := []Candidate{
		{ID: "a", Name: "A", Attributes: map[string]string{
			"complexity": "0.9", "risk": "0.9", "maintainability": "0.9", "performance": "0.9",
		}},
		{ID: "b", Name: "B", Attributes: map[string]string{
			"complexity": "0.8", "risk": "0.8", "maintainability": "0.8", "performance": "0.8",
		}},
	}

	result, err := cr.Analyze(candidates, nil, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if result.NeedsConfirm {
		t.Error("high scores should not need confirmation with low threshold")
	}
}

func TestChoiceResolver_Stats(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	candidates := []Candidate{
		{ID: "a", Name: "A", Attributes: map[string]string{
			"complexity": "0.8", "risk": "0.8", "maintainability": "0.8", "performance": "0.8",
		}},
		{ID: "b", Name: "B", Attributes: map[string]string{
			"complexity": "0.3", "risk": "0.3", "maintainability": "0.3", "performance": "0.3",
		}},
		{ID: "c", Name: "C", Attributes: map[string]string{
			"complexity": "0.5", "risk": "0.5", "maintainability": "0.5", "performance": "0.5",
		}},
	}

	_, _ = cr.Analyze(candidates, nil, nil)
	_, _ = cr.Analyze(candidates[:2], nil, nil)

	stats := cr.Stats()
	if stats.TotalAnalyses != 2 {
		t.Errorf("total analyses = %d, want 2", stats.TotalAnalyses)
	}
	if stats.TotalCandidates != 5 {
		t.Errorf("total candidates = %d, want 5", stats.TotalCandidates)
	}
}

func TestChoiceResolver_Config(t *testing.T) {
	cfg := DefaultChoiceConfig()
	cr := NewChoiceResolver(cfg)
	got := cr.Config()
	if len(got.DefaultCriteria) != len(cfg.DefaultCriteria) {
		t.Error("config criteria count mismatch")
	}
}

func TestChoiceResolver_Justification(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	candidates := []Candidate{
		{ID: "a", Name: "Winner", Attributes: map[string]string{
			"complexity": "0.9", "risk": "0.9", "maintainability": "0.9", "performance": "0.9",
		}},
		{ID: "b", Name: "Loser", Attributes: map[string]string{
			"complexity": "0.1", "risk": "0.1", "maintainability": "0.1", "performance": "0.1",
		}},
	}

	result, err := cr.Analyze(candidates, nil, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if result.Justification == "" {
		t.Error("overall justification should not be empty")
	}
	if result.Rankings[0].Justification == "" {
		t.Error("candidate justification should not be empty")
	}
}

func TestChoiceResolver_EqualScores(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	candidates := []Candidate{
		{ID: "a", Name: "A", Attributes: map[string]string{
			"complexity": "0.5", "risk": "0.5", "maintainability": "0.5", "performance": "0.5",
		}},
		{ID: "b", Name: "B", Attributes: map[string]string{
			"complexity": "0.5", "risk": "0.5", "maintainability": "0.5", "performance": "0.5",
		}},
	}

	result, err := cr.Analyze(candidates, nil, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(result.Rankings) != 2 {
		t.Fatalf("rankings = %d, want 2", len(result.Rankings))
	}
	// Both should have same score.
	if result.Rankings[0].TotalScore != result.Rankings[1].TotalScore {
		t.Errorf("equal candidates should have same score: %v vs %v",
			result.Rankings[0].TotalScore, result.Rankings[1].TotalScore)
	}
}

func TestChoiceResolver_MissingAttributes(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	candidates := []Candidate{
		{ID: "a", Name: "A"}, // No attributes → defaults to 0.5
		{ID: "b", Name: "B", Attributes: map[string]string{
			"complexity": "0.9", "risk": "0.9", "maintainability": "0.9", "performance": "0.9",
		}},
	}

	result, err := cr.Analyze(candidates, nil, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// B should win because all scores are 0.9 vs A's default 0.5.
	if result.RecommendedID != "b" {
		t.Errorf("recommended = %q, want %q", result.RecommendedID, "b")
	}
}

func TestChoiceResolver_ZeroWeights(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	criteria := []Criterion{
		{Name: "a", Weight: 0},
		{Name: "b", Weight: 0},
	}

	candidates := []Candidate{
		{ID: "x", Name: "X"},
		{ID: "y", Name: "Y"},
	}

	result, err := cr.Analyze(candidates, criteria, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}
	// With zero weights → equal weighting fallback.
	if len(result.Rankings) != 2 {
		t.Fatalf("rankings = %d, want 2", len(result.Rankings))
	}
}

func TestChoiceResolver_NegativeWeight(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	criteria := []Criterion{
		{Name: "good", Weight: 1.0},
		{Name: "bad", Weight: -0.5}, // Negative → clamped to 0
	}

	candidates := []Candidate{
		{ID: "a", Name: "A", Attributes: map[string]string{"good": "0.8", "bad": "0.1"}},
		{ID: "b", Name: "B", Attributes: map[string]string{"good": "0.3", "bad": "0.9"}},
	}

	result, err := cr.Analyze(candidates, criteria, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// "good" has all the weight; "bad" clamped to 0.
	if result.RecommendedID != "a" {
		t.Errorf("recommended = %q, want %q", result.RecommendedID, "a")
	}
}

func TestChoiceResolver_SingleCandidate_CustomMinCandidates(t *testing.T) {
	cfg := DefaultChoiceConfig()
	cfg.MinCandidates = 1
	cr := NewChoiceResolver(cfg)

	candidates := []Candidate{
		{ID: "solo", Name: "Solo", Attributes: map[string]string{
			"complexity": "0.7", "risk": "0.7", "maintainability": "0.7", "performance": "0.7",
		}},
	}

	result, err := cr.Analyze(candidates, nil, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if result.RecommendedID != "solo" {
		t.Errorf("recommended = %q, want %q", result.RecommendedID, "solo")
	}
	if len(result.Rankings) != 1 {
		t.Fatalf("rankings = %d, want 1", len(result.Rankings))
	}
}

func TestChoiceResolver_ManyCandidates(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	candidates := make([]Candidate, 10)
	for i := range candidates {
		score := fmt.Sprintf("%.1f", float64(i)/10.0)
		candidates[i] = Candidate{
			ID:   fmt.Sprintf("c%d", i),
			Name: fmt.Sprintf("Candidate %d", i),
			Attributes: map[string]string{
				"complexity":      score,
				"risk":            score,
				"maintainability": score,
				"performance":     score,
			},
		}
	}

	result, err := cr.Analyze(candidates, nil, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if result.RecommendedID != "c9" {
		t.Errorf("recommended = %q, want %q", result.RecommendedID, "c9")
	}
	for i, r := range result.Rankings {
		if r.Rank != i+1 {
			t.Errorf("rank[%d] = %d, want %d", i, r.Rank, i+1)
		}
	}
}

// --- Helper function tests ---

func TestNormalizeCriteriaWeights(t *testing.T) {
	criteria := []Criterion{
		{Name: "a", Weight: 2.0},
		{Name: "b", Weight: 3.0},
	}
	weights := normalizeCriteriaWeights(criteria)
	if len(weights) != 2 {
		t.Fatalf("weights = %d, want 2", len(weights))
	}
	if weights[0] != 0.4 {
		t.Errorf("weight[0] = %v, want 0.4", weights[0])
	}
	if weights[1] != 0.6 {
		t.Errorf("weight[1] = %v, want 0.6", weights[1])
	}
}

func TestNormalizeCriteriaWeights_AllZero(t *testing.T) {
	criteria := []Criterion{
		{Name: "a", Weight: 0},
		{Name: "b", Weight: 0},
	}
	weights := normalizeCriteriaWeights(criteria)
	for _, w := range weights {
		if w != 0.5 {
			t.Errorf("zero-weight fallback = %v, want 0.5", w)
		}
	}
}

func TestClampScore(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{-1.0, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{2.0, 1.0},
	}
	for _, tt := range tests {
		if got := clampScore(tt.in); got != tt.want {
			t.Errorf("clampScore(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestClampScore_NaN(t *testing.T) {
	// NaN and Inf should clamp to 0.5.
	if got := clampScore(math.NaN()); got != 0.5 {
		t.Errorf("clampScore(NaN) = %v, want 0.5", got)
	}
}

func TestAttributeScoreFunc(t *testing.T) {
	cand := Candidate{Attributes: map[string]string{"speed": "0.75"}}
	crit := Criterion{Name: "speed"}
	if got := attributeScoreFunc(cand, crit); got != 0.75 {
		t.Errorf("score = %v, want 0.75", got)
	}
}

func TestAttributeScoreFunc_Missing(t *testing.T) {
	cand := Candidate{Attributes: map[string]string{}}
	crit := Criterion{Name: "speed"}
	if got := attributeScoreFunc(cand, crit); got != 0.5 {
		t.Errorf("missing attribute score = %v, want 0.5", got)
	}
}

func TestAttributeScoreFunc_NilAttributes(t *testing.T) {
	cand := Candidate{}
	crit := Criterion{Name: "speed"}
	if got := attributeScoreFunc(cand, crit); got != 0.5 {
		t.Errorf("nil attributes score = %v, want 0.5", got)
	}
}

func TestAttributeScoreFunc_InvalidFloat(t *testing.T) {
	cand := Candidate{Attributes: map[string]string{"speed": "fast"}}
	crit := Criterion{Name: "speed"}
	if got := attributeScoreFunc(cand, crit); got != 0.5 {
		t.Errorf("invalid float score = %v, want 0.5", got)
	}
}

func TestBuildOverallJustification(t *testing.T) {
	rankings := []CandidateScore{
		{Name: "Winner", TotalScore: 0.9},
		{Name: "Loser", TotalScore: 0.3},
	}
	j := buildOverallJustification(rankings)
	if j == "" {
		t.Error("justification should not be empty")
	}
	if !containsStr(j, "Winner") {
		t.Errorf("justification %q missing Winner", j)
	}
}

func TestBuildOverallJustification_Empty(t *testing.T) {
	j := buildOverallJustification(nil)
	if j != "no candidates" {
		t.Errorf("empty justification = %q, want 'no candidates'", j)
	}
}

func TestBuildCandidateJustification(t *testing.T) {
	cand := Candidate{Name: "Test"}
	scores := map[string]float64{"speed": 0.8}
	criteria := []Criterion{{Name: "speed"}}
	j := buildCandidateJustification(cand, scores, criteria)
	if !containsStr(j, "Test") || !containsStr(j, "speed=0.80") {
		t.Errorf("candidate justification = %q, expected 'Test' and 'speed=0.80'", j)
	}
}

// --- Concurrent Access ---

func TestChoiceResolver_ConcurrentAnalyze(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	candidates := []Candidate{
		{ID: "a", Name: "A", Attributes: map[string]string{
			"complexity": "0.8", "risk": "0.8", "maintainability": "0.8", "performance": "0.8",
		}},
		{ID: "b", Name: "B", Attributes: map[string]string{
			"complexity": "0.3", "risk": "0.3", "maintainability": "0.3", "performance": "0.3",
		}},
	}

	var wg sync.WaitGroup
	const n = 50
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = cr.Analyze(candidates, nil, nil)
		}()
	}
	wg.Wait()

	stats := cr.Stats()
	if stats.TotalAnalyses != n {
		t.Errorf("total analyses = %d, want %d", stats.TotalAnalyses, n)
	}
}

func TestChoiceResolver_ConcurrentStats(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	candidates := []Candidate{
		{ID: "a", Name: "A"},
		{ID: "b", Name: "B"},
	}

	var wg sync.WaitGroup
	wg.Add(30)
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			_, _ = cr.Analyze(candidates, nil, nil)
		}()
	}
	for i := 0; i < 10; i++ {
		go func() {
			defer wg.Done()
			_ = cr.Stats()
		}()
	}
	wg.Wait()

	stats := cr.Stats()
	if stats.TotalAnalyses != 20 {
		t.Errorf("total analyses = %d, want 20", stats.TotalAnalyses)
	}
}

func TestChoiceResolver_PerCriterionScores(t *testing.T) {
	cr := NewChoiceResolver(DefaultChoiceConfig())

	candidates := []Candidate{
		{ID: "a", Name: "A", Attributes: map[string]string{
			"complexity": "0.9", "risk": "0.1", "maintainability": "0.5", "performance": "0.5",
		}},
		{ID: "b", Name: "B", Attributes: map[string]string{
			"complexity": "0.1", "risk": "0.9", "maintainability": "0.5", "performance": "0.5",
		}},
	}

	result, err := cr.Analyze(candidates, nil, nil)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Check that per-criterion scores are accessible.
	for _, r := range result.Rankings {
		if len(r.Scores) != 4 {
			t.Errorf("%s: expected 4 scores, got %d", r.CandidateID, len(r.Scores))
		}
	}
}
