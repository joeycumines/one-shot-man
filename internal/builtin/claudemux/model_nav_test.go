package claudemux

import (
	"errors"
	"strings"
	"testing"
)

func TestParseModelMenu_ArrowSelection(t *testing.T) {
	t.Parallel()

	lines := []string{
		"? Select a model:",
		"❯ claude-sonnet-4-20250514",
		"  claude-3-5-haiku-20241022",
		"  claude-3-opus-20240229",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.Models[0] != "claude-sonnet-4-20250514" {
		t.Errorf("expected first model 'claude-sonnet-4-20250514', got %q", menu.Models[0])
	}
	if menu.Models[1] != "claude-3-5-haiku-20241022" {
		t.Errorf("expected second model 'claude-3-5-haiku-20241022', got %q", menu.Models[1])
	}
	if menu.Models[2] != "claude-3-opus-20240229" {
		t.Errorf("expected third model 'claude-3-opus-20240229', got %q", menu.Models[2])
	}
	if menu.SelectedIndex != 0 {
		t.Errorf("expected SelectedIndex=0, got %d", menu.SelectedIndex)
	}
}

func TestParseModelMenu_GreaterThanSelection(t *testing.T) {
	t.Parallel()

	lines := []string{
		"? Which model would you like to use?",
		"  llama3.2",
		"> codellama:7b",
		"  mistral:latest",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.SelectedIndex != 1 {
		t.Errorf("expected SelectedIndex=1 (codellama:7b), got %d", menu.SelectedIndex)
	}
	if menu.Models[1] != "codellama:7b" {
		t.Errorf("expected selected model 'codellama:7b', got %q", menu.Models[1])
	}
}

func TestParseModelMenu_NumberedList_Parens(t *testing.T) {
	t.Parallel()

	lines := []string{
		"Available models:",
		"1) llama3.2",
		"2) codellama:7b",
		"3) mistral:latest",
		"Enter selection [1-3]:",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.Models[0] != "llama3.2" {
		t.Errorf("expected first model 'llama3.2', got %q", menu.Models[0])
	}
	if menu.Models[2] != "mistral:latest" {
		t.Errorf("expected third model 'mistral:latest', got %q", menu.Models[2])
	}
	if menu.SelectedIndex != -1 {
		t.Errorf("numbered list should have SelectedIndex=-1, got %d", menu.SelectedIndex)
	}
}

func TestParseModelMenu_NumberedList_Dots(t *testing.T) {
	t.Parallel()

	lines := []string{
		"Models:",
		"1. claude-sonnet-4-20250514",
		"2. claude-opus",
		"Select:",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 2 {
		t.Fatalf("expected 2 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.Models[0] != "claude-sonnet-4-20250514" {
		t.Errorf("expected first model 'claude-sonnet-4-20250514', got %q", menu.Models[0])
	}
}

func TestParseModelMenu_SingleModel(t *testing.T) {
	t.Parallel()

	lines := []string{
		"Select a model:",
		"❯ claude-sonnet-4-20250514",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 1 {
		t.Fatalf("expected 1 model, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.SelectedIndex != 0 {
		t.Errorf("expected SelectedIndex=0, got %d", menu.SelectedIndex)
	}
}

func TestParseModelMenu_Empty(t *testing.T) {
	t.Parallel()

	menu := ParseModelMenu(nil)
	if len(menu.Models) != 0 {
		t.Errorf("expected 0 models for nil input, got %d", len(menu.Models))
	}
	if menu.SelectedIndex != -1 {
		t.Errorf("expected SelectedIndex=-1, got %d", menu.SelectedIndex)
	}

	menu = ParseModelMenu([]string{})
	if len(menu.Models) != 0 {
		t.Errorf("expected 0 models for empty input, got %d", len(menu.Models))
	}
}

func TestParseModelMenu_NoMatchLines(t *testing.T) {
	t.Parallel()

	lines := []string{
		"This is just some regular text",
		"with no model selection indicators",
		"func main() {",
	}

	menu := ParseModelMenu(lines)
	if len(menu.Models) != 0 {
		t.Errorf("expected 0 models from non-menu text, got %d: %v", len(menu.Models), menu.Models)
	}
}

func TestParseModelMenu_SelectedMiddle(t *testing.T) {
	t.Parallel()

	lines := []string{
		"  model-a",
		"  model-b",
		"❯ model-c",
		"  model-d",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 4 {
		t.Fatalf("expected 4 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.SelectedIndex != 2 {
		t.Errorf("expected SelectedIndex=2 (model-c), got %d", menu.SelectedIndex)
	}
}

func TestParseModelMenu_MixedFormats(t *testing.T) {
	t.Parallel()

	// Numbered items should be detected regardless of preceding context.
	lines := []string{
		"Choose a model:",
		"1) alpha",
		"2) beta",
		"> gamma",
		"  delta",
	}

	menu := ParseModelMenu(lines)
	// Numbered items first, then arrow-selected, then indented.
	if len(menu.Models) != 4 {
		t.Fatalf("expected 4 models, got %d: %v", len(menu.Models), menu.Models)
	}
}

func TestParseModelMenu_TrailingWhitespace(t *testing.T) {
	t.Parallel()

	lines := []string{
		"❯ model-with-trailing   ",
		"  model-with-trailing-too   ",
	}

	menu := ParseModelMenu(lines)
	if len(menu.Models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(menu.Models))
	}
	if menu.Models[0] != "model-with-trailing" {
		t.Errorf("expected trimmed model name, got %q", menu.Models[0])
	}
	if menu.Models[1] != "model-with-trailing-too" {
		t.Errorf("expected trimmed model name, got %q", menu.Models[1])
	}
}

// --- NavigateToModel tests ---

func TestNavigateToModel_AlreadySelected(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{
		Models:        []string{"alpha", "beta", "gamma"},
		SelectedIndex: 1,
	}

	keys, err := NavigateToModel(menu, "beta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keys != KeyEnter {
		t.Errorf("expected just Enter, got %q", keys)
	}
}

func TestNavigateToModel_NavigateDown(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{
		Models:        []string{"alpha", "beta", "gamma", "delta"},
		SelectedIndex: 0,
	}

	keys, err := NavigateToModel(menu, "gamma")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := KeyArrowDown + KeyArrowDown + KeyEnter
	if keys != expected {
		t.Errorf("expected %q, got %q", expected, keys)
	}
}

func TestNavigateToModel_NavigateUp(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{
		Models:        []string{"alpha", "beta", "gamma", "delta"},
		SelectedIndex: 3,
	}

	keys, err := NavigateToModel(menu, "alpha")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := KeyArrowUp + KeyArrowUp + KeyArrowUp + KeyEnter
	if keys != expected {
		t.Errorf("expected %q, got %q", expected, keys)
	}
}

func TestNavigateToModel_SingleModel_AutoSelect(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{
		Models:        []string{"only-model"},
		SelectedIndex: 0,
	}

	keys, err := NavigateToModel(menu, "only-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keys != KeyEnter {
		t.Errorf("expected just Enter for single model, got %q", keys)
	}
}

func TestNavigateToModel_SingleModel_DifferentTarget(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{
		Models:        []string{"only-model"},
		SelectedIndex: 0,
	}

	// Single-model menu always returns Enter regardless of target name.
	keys, err := NavigateToModel(menu, "different-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keys != KeyEnter {
		t.Errorf("expected just Enter for single model auto-select, got %q", keys)
	}
}

func TestNavigateToModel_NoModels(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{SelectedIndex: -1}

	_, err := NavigateToModel(menu, "target")
	if !errors.Is(err, ErrNoModels) {
		t.Errorf("expected ErrNoModels, got %v", err)
	}
}

func TestNavigateToModel_ModelNotFound(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{
		Models:        []string{"alpha", "beta", "gamma"},
		SelectedIndex: 0,
	}

	_, err := NavigateToModel(menu, "nonexistent")
	if !errors.Is(err, ErrModelNotFound) {
		t.Errorf("expected ErrModelNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "nonexistent") {
		t.Errorf("error should contain target name, got: %v", err)
	}
	if !strings.Contains(err.Error(), "alpha") {
		t.Errorf("error should list available models, got: %v", err)
	}
}

func TestNavigateToModel_CaseInsensitive(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{
		Models:        []string{"Claude-Sonnet", "Claude-Opus"},
		SelectedIndex: 0,
	}

	keys, err := NavigateToModel(menu, "claude-opus")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := KeyArrowDown + KeyEnter
	if keys != expected {
		t.Errorf("expected %q, got %q", expected, keys)
	}
}

func TestNavigateToModel_SubstringMatch(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{
		Models:        []string{"claude-sonnet-4-20250514", "claude-3-5-haiku-20241022"},
		SelectedIndex: 0,
	}

	keys, err := NavigateToModel(menu, "haiku")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := KeyArrowDown + KeyEnter
	if keys != expected {
		t.Errorf("expected %q, got %q", expected, keys)
	}
}

func TestNavigateToModel_NoSelectedIndex(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{
		Models:        []string{"alpha", "beta", "gamma"},
		SelectedIndex: -1, // No selection indicator found.
	}

	// Should assume first item is selected.
	keys, err := NavigateToModel(menu, "gamma")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := KeyArrowDown + KeyArrowDown + KeyEnter
	if keys != expected {
		t.Errorf("expected %q, got %q", expected, keys)
	}
}

func TestNavigateToModel_TooLong(t *testing.T) {
	t.Parallel()

	// Create a menu with MaxNavigationSteps+2 items.
	models := make([]string, MaxNavigationSteps+2)
	for i := range models {
		models[i] = strings.Repeat("m", i+1)
	}

	menu := &ModelMenu{
		Models:        models,
		SelectedIndex: 0,
	}

	_, err := NavigateToModel(menu, models[len(models)-1])
	if !errors.Is(err, ErrNavigationTooLong) {
		t.Errorf("expected ErrNavigationTooLong, got %v", err)
	}
}

// --- findModelIndex tests ---

func TestFindModelIndex_ExactMatchPrecedence(t *testing.T) {
	t.Parallel()

	models := []string{"haiku", "claude-3-5-haiku-20241022"}

	idx := findModelIndex(models, "haiku")
	if idx != 0 {
		t.Errorf("exact match should return index 0, got %d", idx)
	}
}

func TestFindModelIndex_CaseInsensitiveFallback(t *testing.T) {
	t.Parallel()

	models := []string{"Claude-Sonnet", "Claude-Opus"}

	idx := findModelIndex(models, "CLAUDE-SONNET")
	if idx != 0 {
		t.Errorf("case-insensitive match should return index 0, got %d", idx)
	}
}

func TestFindModelIndex_SubstringFallback(t *testing.T) {
	t.Parallel()

	models := []string{"claude-sonnet-4-20250514", "claude-3-opus-20240229"}

	idx := findModelIndex(models, "opus")
	if idx != 1 {
		t.Errorf("substring match should return index 1, got %d", idx)
	}
}

func TestFindModelIndex_NoMatch(t *testing.T) {
	t.Parallel()

	models := []string{"alpha", "beta"}

	idx := findModelIndex(models, "gamma")
	if idx != -1 {
		t.Errorf("expected -1 for no match, got %d", idx)
	}
}

// --- Representative PTY output samples ---

func TestParseModelMenu_OllamaStyleOutput(t *testing.T) {
	t.Parallel()

	// Simulated ollama model selection TUI output.
	lines := []string{
		"? Which model would you like to use?",
		"> llama3.2",
		"  codellama:7b",
		"  mistral:latest",
		"  llama3.2:1b",
		"  phi3:mini",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 5 {
		t.Fatalf("expected 5 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.SelectedIndex != 0 {
		t.Errorf("expected SelectedIndex=0, got %d", menu.SelectedIndex)
	}
	if menu.Models[0] != "llama3.2" {
		t.Errorf("expected 'llama3.2', got %q", menu.Models[0])
	}
	if menu.Models[4] != "phi3:mini" {
		t.Errorf("expected 'phi3:mini', got %q", menu.Models[4])
	}

	// Navigate to mistral:latest from position 0.
	keys, err := NavigateToModel(menu, "mistral:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := KeyArrowDown + KeyArrowDown + KeyEnter
	if keys != expected {
		t.Errorf("expected 2 down arrows + enter, got %q", keys)
	}
}

func TestParseModelMenu_ClaudeCodeStyleOutput(t *testing.T) {
	t.Parallel()

	// Simulated Claude Code model selection TUI output.
	lines := []string{
		"Select a model",
		"❯ claude-sonnet-4-20250514",
		"  claude-3-5-haiku-20241022",
		"  claude-3-opus-20240229",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.SelectedIndex != 0 {
		t.Errorf("expected SelectedIndex=0, got %d", menu.SelectedIndex)
	}

	// Navigate to opus using substring match.
	keys, err := NavigateToModel(menu, "opus")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := KeyArrowDown + KeyArrowDown + KeyEnter
	if keys != expected {
		t.Errorf("expected 2 down arrows + enter, got %q", keys)
	}
}

func TestParseModelMenu_NumberedListOutput(t *testing.T) {
	t.Parallel()

	// Simulated numbered model selection.
	lines := []string{
		"Available models:",
		"  1) gpt-4o",
		"  2) gpt-4o-mini",
		"  3) gpt-3.5-turbo",
		"Enter selection [1-3]:",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.SelectedIndex != -1 {
		t.Errorf("numbered list should have SelectedIndex=-1, got %d", menu.SelectedIndex)
	}
	if menu.Models[0] != "gpt-4o" {
		t.Errorf("expected 'gpt-4o', got %q", menu.Models[0])
	}
}

func TestParseModelMenu_OllamaTriangleArrow(t *testing.T) {
	t.Parallel()

	// Ollama uses ▸ (U+25B8 BLACK RIGHT-POINTING SMALL TRIANGLE) as selection indicator.
	lines := []string{
		"? Which model would you like to use?",
		"  llama3.2",
		"▸ gpt-oss:20b-cloud",
		"  codellama:7b",
		"  mistral:latest",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 4 {
		t.Fatalf("expected 4 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.SelectedIndex != 1 {
		t.Errorf("expected SelectedIndex=1 (gpt-oss:20b-cloud), got %d", menu.SelectedIndex)
	}
	if menu.Models[1] != "gpt-oss:20b-cloud" {
		t.Errorf("expected selected model 'gpt-oss:20b-cloud', got %q", menu.Models[1])
	}
}

func TestParseModelMenu_RightwardsArrow(t *testing.T) {
	t.Parallel()

	// → (U+2192 RIGHTWARDS ARROW) as selection indicator.
	lines := []string{
		"→ model-alpha",
		"  model-beta",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 2 {
		t.Fatalf("expected 2 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.SelectedIndex != 0 {
		t.Errorf("expected SelectedIndex=0, got %d", menu.SelectedIndex)
	}
}

func TestParseModelMenu_BlackPointer(t *testing.T) {
	t.Parallel()

	// ► (U+25BA BLACK RIGHT-POINTING POINTER) as selection indicator.
	lines := []string{
		"  option-a",
		"► option-b",
		"  option-c",
	}

	menu := ParseModelMenu(lines)

	if len(menu.Models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(menu.Models), menu.Models)
	}
	if menu.SelectedIndex != 1 {
		t.Errorf("expected SelectedIndex=1 (option-b), got %d", menu.SelectedIndex)
	}
}

func TestParseModelMenu_OllamaTriangleArrow_NavigateToModel(t *testing.T) {
	t.Parallel()

	// Full end-to-end: Ollama ▸ indicator + navigate to specific model.
	lines := []string{
		"? Which model would you like to use?",
		"▸ llama3.2",
		"  gpt-oss:20b-cloud",
		"  codellama:7b",
	}

	menu := ParseModelMenu(lines)
	if len(menu.Models) != 3 {
		t.Fatalf("expected 3 models, got %d: %v", len(menu.Models), menu.Models)
	}

	keys, err := NavigateToModel(menu, "gpt-oss:20b-cloud")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := KeyArrowDown + KeyEnter
	if keys != expected {
		t.Errorf("expected 1 down arrow + enter, got %q", keys)
	}
}

// --- Keystroke constant tests ---

func TestKeystrokeConstants(t *testing.T) {
	t.Parallel()

	if KeyArrowUp != "\x1b[A" {
		t.Errorf("KeyArrowUp = %q, want %q", KeyArrowUp, "\x1b[A")
	}
	if KeyArrowDown != "\x1b[B" {
		t.Errorf("KeyArrowDown = %q, want %q", KeyArrowDown, "\x1b[B")
	}
	if KeyEnter != "\r" {
		t.Errorf("KeyEnter = %q, want %q", KeyEnter, "\r")
	}
}

func TestNavigateToModel_ExactMatchPrecedence(t *testing.T) {
	t.Parallel()

	// "sonnet" appears as exact match AND as substring of another model.
	menu := &ModelMenu{
		Models:        []string{"claude-sonnet-4-20250514", "sonnet"},
		SelectedIndex: 0,
	}

	keys, err := NavigateToModel(menu, "sonnet")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should match "sonnet" exactly at index 1, not substring at index 0.
	expected := KeyArrowDown + KeyEnter
	if keys != expected {
		t.Errorf("exact match should be preferred, expected %q, got %q", expected, keys)
	}
}

// --- IsLauncherMenu tests ---

func TestIsLauncherMenu_OllamaLauncher(t *testing.T) {
	t.Parallel()

	// Ollama 0.16.2 launcher menu output (ANSI stripped).
	lines := []string{
		"Ollama 0.16.2",
		"",
		"▸ Run a model",
		"  Start an interactive chat with a model",
		"",
		"  Launch Claude Code",
		"  Agentic coding across large codebases",
		"",
		"  Launch Codex (not installed)",
		"  Launch OpenClaw (not installed)",
		"  More...",
	}

	menu := ParseModelMenu(lines)
	if !IsLauncherMenu(menu) {
		t.Error("expected IsLauncherMenu=true for Ollama launcher, got false")
		t.Logf("Parsed items: %v", menu.Models)
	}
}

func TestIsLauncherMenu_RealModelMenu(t *testing.T) {
	t.Parallel()

	// Real model selection menu — should NOT be detected as launcher.
	lines := []string{
		"? Which model would you like to use?",
		"▸ gpt-oss:20b-cloud",
		"  glm-4.7-flash:latest",
		"  codellama:7b",
	}

	menu := ParseModelMenu(lines)
	if IsLauncherMenu(menu) {
		t.Error("expected IsLauncherMenu=false for real model menu, got true")
	}
}

func TestIsLauncherMenu_EmptyMenu(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{SelectedIndex: -1}
	if IsLauncherMenu(menu) {
		t.Error("expected IsLauncherMenu=false for empty menu, got true")
	}
}

func TestIsLauncherMenu_CaseInsensitive(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{
		Models:        []string{"RUN A MODEL", "Launch Claude Code"},
		SelectedIndex: 0,
	}
	if !IsLauncherMenu(menu) {
		t.Error("expected case-insensitive detection of launcher menu")
	}
}

// --- DismissLauncherKeys tests ---

func TestDismissLauncherKeys_AlreadySelected(t *testing.T) {
	t.Parallel()

	// "Run a model" is already selected (index 0).
	menu := &ModelMenu{
		Models:        []string{"Run a model", "Launch Claude Code", "More..."},
		SelectedIndex: 0,
	}

	keys := DismissLauncherKeys(menu)
	if keys != KeyEnter {
		t.Errorf("expected just Enter when already on 'Run a model', got %q", keys)
	}
}

func TestDismissLauncherKeys_NeedNavigate(t *testing.T) {
	t.Parallel()

	// "Launch Claude Code" is selected (index 1), need to go UP to "Run a model" (index 0).
	menu := &ModelMenu{
		Models:        []string{"Run a model", "Launch Claude Code", "More..."},
		SelectedIndex: 1,
	}

	keys := DismissLauncherKeys(menu)
	expected := KeyArrowUp + KeyEnter
	if keys != expected {
		t.Errorf("expected Up+Enter, got %q", keys)
	}
}

func TestDismissLauncherKeys_EmptyMenu(t *testing.T) {
	t.Parallel()

	menu := &ModelMenu{SelectedIndex: -1}
	keys := DismissLauncherKeys(menu)
	if keys != "" {
		t.Errorf("expected empty string for empty menu, got %q", keys)
	}
}

func TestDismissLauncherKeys_NoRunItem(t *testing.T) {
	t.Parallel()

	// No "Run a model" item — should still return Enter as fallback.
	menu := &ModelMenu{
		Models:        []string{"Launch Claude Code", "More..."},
		SelectedIndex: 0,
	}

	keys := DismissLauncherKeys(menu)
	if keys != KeyEnter {
		t.Errorf("expected Enter fallback when 'Run a model' not found, got %q", keys)
	}
}

func TestParseModelMenu_OllamaLauncherOutput(t *testing.T) {
	t.Parallel()

	// Real Ollama 0.16.2 output (ANSI escape sequences stripped, as the
	// pipeline would do before feeding to ParseModelMenu in production).
	lines := []string{
		"Ollama 0.16.2",
		"",
		"▸ Run a model",
		"  Start an interactive chat with a model",
		"",
		"  Launch Claude Code",
		"  Agentic coding across large codebases",
		"",
		"  Launch Codex (not installed)",
		"  OpenAI's open-source coding agent",
		"",
		"  Launch OpenClaw (not installed)",
		"  Personal AI with 100+ skills",
		"",
		"  More...",
		"  Show additional integrations",
	}

	menu := ParseModelMenu(lines)

	// Should detect menu items (those matching selection/indented patterns).
	if len(menu.Models) == 0 {
		t.Fatal("expected at least 1 model item from launcher output")
	}

	// First item should be "Run a model" (matched by ▸).
	if menu.Models[0] != "Run a model" {
		t.Errorf("expected first item 'Run a model', got %q", menu.Models[0])
	}

	// Should be detected as launcher.
	if !IsLauncherMenu(menu) {
		t.Error("expected IsLauncherMenu=true for parsed launcher output")
	}

	// DismissLauncherKeys should return Enter (already selected).
	if menu.SelectedIndex != 0 {
		t.Errorf("expected SelectedIndex=0, got %d", menu.SelectedIndex)
	}
	keys := DismissLauncherKeys(menu)
	if keys != KeyEnter {
		t.Errorf("expected just Enter to dismiss, got %q", keys)
	}
}

// --- StripANSI tests ---

func TestStripANSI_Basic(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"color code", "\x1b[31mred\x1b[0m", "red"},
		{"bold+bg", "\x1b[1;48;5;236m▸ Run a model\x1b[0m", "▸ Run a model"},
		{"cursor hide", "\x1b[?25l", ""},
		{"erase to end", "text\x1b[K", "text"},
		{"256 color", "\x1b[38;5;246mgreyed out\x1b[0m", "greyed out"},
		{"italic", "\x1b[3;38;5;246m(not installed)\x1b[0m", "(not installed)"},
		{"multiple codes", "\x1b[1mOllama \x1b[38;5;250m0.16.2\x1b[0m\x1b[0m", "Ollama 0.16.2"},
		{"empty", "", ""},
		{"no ansi", "just text", "just text"},
		{"bracketed paste", "\x1b[?2004h", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := StripANSI(tt.input)
			if got != tt.want {
				t.Errorf("StripANSI(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseModelMenu_OllamaLauncherWithANSI(t *testing.T) {
	t.Parallel()

	// Real Ollama 0.16.2 output WITH ANSI escape sequences (captured from PTY).
	lines := []string{
		"\x1b[?25l",
		"\x1b[?2004h",
		"\x1b[1mOllama \x1b[38;5;250m0.16.2\x1b[0m\x1b[0m                                          \x1b[K",
		"                                                       \x1b[K",
		"\x1b[1;48;5;236m▸ Run a model\x1b[0m                                          \x1b[K",
		"    \x1b[38;5;246mStart an interactive chat with a model\x1b[0m             \x1b[K",
		"                                                       \x1b[K",
		"  Launch Claude Code                                   \x1b[K",
		"    \x1b[38;5;246mAgentic coding across large codebases\x1b[0m              \x1b[K",
		"                                                       \x1b[K",
		"  \x1b[38;5;246mLaunch Codex \x1b[3;38;5;246m(not installed)\x1b[0m\x1b[0m                         \x1b[K",
		"    \x1b[38;5;246mOpenAI's open-source coding agent\x1b[0m                  \x1b[K",
		"                                                       \x1b[K",
		"  \x1b[38;5;246mLaunch OpenClaw \x1b[3;38;5;246m(not installed)\x1b[0m\x1b[0m                      \x1b[K",
		"    \x1b[38;5;246mPersonal AI with 100+ skills\x1b[0m                       \x1b[K",
		"                                                       \x1b[K",
		"  More...                                              \x1b[K",
		"    \x1b[38;5;246mShow additional integrations\x1b[0m                       \x1b[K",
		"                                                       \x1b[K",
		"                                                       \x1b[K",
		"\x1b[38;5;244m↑/↓ navigate • enter launch • → change model • esc quit\x1b[0m\x1b[K",
	}

	menu := ParseModelMenu(lines)

	// With ANSI stripping in ParseModelMenu, this should now detect the launcher.
	if len(menu.Models) == 0 {
		t.Fatal("expected at least 1 model item from ANSI launcher output")
	}

	// First item should be "Run a model".
	if menu.Models[0] != "Run a model" {
		t.Errorf("expected first item 'Run a model', got %q", menu.Models[0])
	}

	// Should be detected as launcher.
	if !IsLauncherMenu(menu) {
		t.Error("expected IsLauncherMenu=true for ANSI launcher output")
	}

	// Should have more items parsed (the indented ones).
	if len(menu.Models) < 3 {
		t.Logf("Parsed models: %v", menu.Models)
		t.Errorf("expected at least 3 items from launcher, got %d", len(menu.Models))
	}
}
