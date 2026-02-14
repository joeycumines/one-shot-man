package command

import (
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestConfigHotSnippetsForJS(t *testing.T) {
	t.Run("NilConfig", func(t *testing.T) {
		result := configHotSnippetsForJS(nil)
		if result != nil {
			t.Errorf("expected nil for nil config, got %v", result)
		}
	})

	t.Run("EmptySnippets", func(t *testing.T) {
		cfg := config.NewConfig()
		result := configHotSnippetsForJS(cfg)
		if result != nil {
			t.Errorf("expected nil for empty snippets, got %v", result)
		}
	})

	t.Run("SingleSnippet", func(t *testing.T) {
		cfg := config.NewConfig()
		cfg.HotSnippets = []config.HotSnippet{
			{Name: "followup", Text: "Continue with context."},
		}
		result := configHotSnippetsForJS(cfg)
		if len(result) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(result))
		}
		if result[0]["name"] != "followup" {
			t.Errorf("name = %v, want %q", result[0]["name"], "followup")
		}
		if result[0]["text"] != "Continue with context." {
			t.Errorf("text = %v, want %q", result[0]["text"], "Continue with context.")
		}
		if _, exists := result[0]["description"]; exists {
			t.Error("description should not be present when empty")
		}
	})

	t.Run("SnippetWithDescription", func(t *testing.T) {
		cfg := config.NewConfig()
		cfg.HotSnippets = []config.HotSnippet{
			{Name: "followup", Text: "Continue.", Description: "Follow-up prompt"},
		}
		result := configHotSnippetsForJS(cfg)
		if len(result) != 1 {
			t.Fatalf("expected 1 entry, got %d", len(result))
		}
		if result[0]["description"] != "Follow-up prompt" {
			t.Errorf("description = %v, want %q", result[0]["description"], "Follow-up prompt")
		}
	})

	t.Run("MultipleSnippets", func(t *testing.T) {
		cfg := config.NewConfig()
		cfg.HotSnippets = []config.HotSnippet{
			{Name: "followup", Text: "Continue."},
			{Name: "kickoff", Text: "You are an expert.", Description: "Kickoff prompt"},
		}
		result := configHotSnippetsForJS(cfg)
		if len(result) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(result))
		}
		if result[0]["name"] != "followup" {
			t.Errorf("entry 0 name = %v, want %q", result[0]["name"], "followup")
		}
		if result[1]["name"] != "kickoff" {
			t.Errorf("entry 1 name = %v, want %q", result[1]["name"], "kickoff")
		}
		if result[1]["description"] != "Kickoff prompt" {
			t.Errorf("entry 1 description = %v, want %q", result[1]["description"], "Kickoff prompt")
		}
	})

	t.Run("NewlinePreserved", func(t *testing.T) {
		cfg := config.NewConfig()
		cfg.HotSnippets = []config.HotSnippet{
			{Name: "multi", Text: "Line 1\nLine 2"},
		}
		result := configHotSnippetsForJS(cfg)
		if result[0]["text"] != "Line 1\nLine 2" {
			t.Errorf("text = %q, want %q", result[0]["text"], "Line 1\nLine 2")
		}
	})
}
