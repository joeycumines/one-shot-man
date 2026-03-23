package command

// T424: Unit tests for chunk 16b _formatReportForDisplay.
//
// Pure function: report object → formatted multi-line text.
// Covers: null report, empty report, metadata defaults, analysis section
// with files/statuses, groups section, plan section with splits,
// equivalence section (verified/not), missing optional sections.

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestChunk16b_FormatReport_NullReport(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	for _, input := range []string{"null", "undefined", "false", "0", "''"} {
		t.Run(input, func(t *testing.T) {
			val, err := evalJS(`prSplit._formatReportForDisplay(` + input + `)`)
			if err != nil {
				t.Fatal(err)
			}
			s, _ := val.(string)
			if !strings.Contains(s, "Report generation failed") {
				t.Errorf("falsy input %s should produce error message, got: %q", input, s)
			}
			if !strings.Contains(s, "Press Esc to close") {
				t.Errorf("falsy input %s should contain Esc instruction, got: %q", input, s)
			}
		})
	}
}

func TestChunk16b_FormatReport_EmptyReport(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Empty object: metadata defaults, no optional sections.
	val, err := evalJS(`prSplit._formatReportForDisplay({})`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	checks := []struct {
		contains string
		desc     string
	}{
		{"PR Split Report", "header"},
		{"Version:    unknown", "version default"},
		{`Base:       —`, "base default"},
		{`Strategy:   —`, "strategy default"},
		{"Dry Run:    no", "dryRun default"},
		{"Press c to copy", "footer"},
	}
	for _, c := range checks {
		if !strings.Contains(s, c.contains) {
			t.Errorf("missing %s (%q) in output:\n%s", c.desc, c.contains, s)
		}
	}
	// No section headers for optional sections.
	for _, absent := range []string{"Analysis", "Groups", "Split Plan", "Equivalence"} {
		if strings.Contains(s, absent) {
			t.Errorf("empty report should not have %s section:\n%s", absent, s)
		}
	}
}

func TestChunk16b_FormatReport_MetadataPopulated(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._formatReportForDisplay({
		version: '2.1.0',
		baseBranch: 'main',
		strategy: 'semantic',
		dryRun: true,
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	checks := map[string]string{
		"Version:    2.1.0":    "version",
		"Base:       main":     "base",
		"Strategy:   semantic": "strategy",
		"Dry Run:    yes":      "dryRun",
	}
	for expected, desc := range checks {
		if !strings.Contains(s, expected) {
			t.Errorf("missing %s (%q) in output:\n%s", desc, expected, s)
		}
	}
}

func TestChunk16b_FormatReport_AnalysisSection(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._formatReportForDisplay({
		analysis: {
			currentBranch: 'feature/xyz',
			baseBranch: 'develop',
			fileCount: 3,
			files: ['a.go', 'b.go', 'c.go'],
			fileStatuses: {'a.go': 'modified', 'c.go': 'added'},
		},
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	checks := []struct {
		contains string
		desc     string
	}{
		{"Analysis", "section header"},
		{"Source Branch:  feature/xyz", "current branch"},
		{"Base Branch:    develop", "analysis base branch"},
		{"File Count:     3", "file count"},
		{"a.go (modified)", "file with status"},
		{"b.go", "file without status"},
		{"c.go (added)", "file with added status"},
	}
	for _, c := range checks {
		if !strings.Contains(s, c.contains) {
			t.Errorf("missing %s (%q) in output:\n%s", c.desc, c.contains, s)
		}
	}
	// b.go should NOT have a status.
	if strings.Contains(s, "b.go (") {
		t.Errorf("b.go should not have status parenthetical:\n%s", s)
	}
}

func TestChunk16b_FormatReport_AnalysisNoFiles(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Analysis with no files array.
	val, err := evalJS(`prSplit._formatReportForDisplay({
		analysis: {currentBranch: 'feat', fileCount: 0},
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)
	if !strings.Contains(s, "Analysis") {
		t.Errorf("should have Analysis section:\n%s", s)
	}
	if !strings.Contains(s, "File Count:     0") {
		t.Errorf("file count should be 0:\n%s", s)
	}
}

func TestChunk16b_FormatReport_GroupsSection(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._formatReportForDisplay({
		groups: [
			{name: 'core', files: ['x.go', 'y.go']},
			{name: null, files: ['z.go']},
			null,
			{name: 'empty'},
		],
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	checks := []struct {
		contains string
		desc     string
	}{
		{"Groups", "section header"},
		{"core (2 files)", "named group with count"},
		{"x.go", "group file 1"},
		{"y.go", "group file 2"},
		{"(unnamed) (1 files)", "unnamed group"},
		{"z.go", "unnamed group file"},
		{"empty (0 files)", "group with no files array"},
	}
	for _, c := range checks {
		if !strings.Contains(s, c.contains) {
			t.Errorf("missing %s (%q) in output:\n%s", c.desc, c.contains, s)
		}
	}
}

func TestChunk16b_FormatReport_PlanSection(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._formatReportForDisplay({
		plan: {
			splitCount: 2,
			splits: [
				{name: 'split-a', message: 'Add feature A', files: ['a1.go', 'a2.go']},
				{name: 'split-b', message: null, files: []},
			],
		},
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	checks := []struct {
		contains string
		desc     string
	}{
		{"Split Plan (2 splits)", "plan header with count"},
		{"1. split-a", "split 1 name"},
		{"Message:  Add feature A", "split 1 message"},
		{"Files:    2", "split 1 file count"},
		{"a1.go", "split 1 file 1"},
		{"a2.go", "split 1 file 2"},
		{"2. split-b", "split 2 name"},
		{`Message:  —`, "split 2 null message default"},
		{"Files:    0", "split 2 empty files"},
	}
	for _, c := range checks {
		if !strings.Contains(s, c.contains) {
			t.Errorf("missing %s (%q) in output:\n%s", c.desc, c.contains, s)
		}
	}
}

func TestChunk16b_FormatReport_EquivalenceVerified(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._formatReportForDisplay({
		equivalence: {
			verified: true,
			splitTree: 'abc123',
			sourceTree: 'def456',
		},
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	checks := []struct {
		contains string
		desc     string
	}{
		{"Equivalence Check", "section header"},
		{"Verified:     YES", "verified true"},
		{"Split Tree:   abc123", "split tree"},
		{"Source Tree:  def456", "source tree"},
	}
	for _, c := range checks {
		if !strings.Contains(s, c.contains) {
			t.Errorf("missing %s (%q) in output:\n%s", c.desc, c.contains, s)
		}
	}
	if strings.Contains(s, "Error:") {
		t.Errorf("should not have Error line when not set:\n%s", s)
	}
}

func TestChunk16b_FormatReport_EquivalenceNotVerified(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	val, err := evalJS(`prSplit._formatReportForDisplay({
		equivalence: {
			verified: false,
			error: 'trees do not match',
		},
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	if !strings.Contains(s, "NO") {
		t.Errorf("should show NO for unverified:\n%s", s)
	}
	if !strings.Contains(s, "Error:        trees do not match") {
		t.Errorf("should show error text:\n%s", s)
	}
	// Should NOT have splitTree/sourceTree lines.
	if strings.Contains(s, "Split Tree:") || strings.Contains(s, "Source Tree:") {
		t.Errorf("should not show tree hashes when absent:\n%s", s)
	}
}

func TestChunk16b_FormatReport_FullReport(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngine(t)

	// Full report with all sections populated.
	val, err := evalJS(`prSplit._formatReportForDisplay({
		version: '1.0.0',
		baseBranch: 'main',
		strategy: 'directory',
		dryRun: false,
		analysis: {
			currentBranch: 'feat/x',
			baseBranch: 'main',
			fileCount: 2,
			files: ['a.go', 'b.go'],
			fileStatuses: {'a.go': 'modified'},
		},
		groups: [{name: 'g1', files: ['a.go']}],
		plan: {
			splitCount: 1,
			splits: [{name: 's1', message: 'msg', files: ['a.go']}],
		},
		equivalence: {verified: true, splitTree: 'aaa', sourceTree: 'bbb'},
	})`)
	if err != nil {
		t.Fatal(err)
	}
	s, _ := val.(string)

	// All sections should appear in order.
	sections := []string{
		"PR Split Report", "Version:", "Analysis", "Groups", "Split Plan", "Equivalence", "Press c to copy",
	}
	lastIdx := -1
	for _, sec := range sections {
		idx := strings.Index(s, sec)
		if idx < 0 {
			t.Errorf("missing section %q in full report:\n%s", sec, s)
			continue
		}
		if idx <= lastIdx {
			t.Errorf("section %q out of order (at %d, prev at %d):\n%s", sec, idx, lastIdx, s)
		}
		lastIdx = idx
	}
}
