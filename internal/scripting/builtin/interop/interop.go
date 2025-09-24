package interop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/require"
)

// SharedContext represents data that can be shared between modes
type SharedContext struct {
	// Metadata
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Version     string    `json:"version"`
	SourceMode  string    `json:"source_mode"`
	TargetModes []string  `json:"target_modes,omitempty"`

	// Common context items (matches the structure used by both modes)
	ContextItems []ContextItem `json:"context_items"`

	// Mode-specific data
	CodeReview  *CodeReviewData  `json:"code_review,omitempty"`
	PromptFlow  *PromptFlowData  `json:"prompt_flow,omitempty"`
	CommitGen   *CommitGenData   `json:"commit_gen,omitempty"`
}

// ContextItem represents a shared context item (file, diff, note, etc.)
type ContextItem struct {
	ID      int    `json:"id"`
	Type    string `json:"type"`    // file, diff, lazy-diff, note
	Label   string `json:"label"`
	Payload string `json:"payload"` // can be content, args for lazy-diff, etc.
}

// CodeReviewData holds code review specific data
type CodeReviewData struct {
	Template string `json:"template,omitempty"`
	Prompt   string `json:"prompt,omitempty"`
}

// PromptFlowData holds prompt flow specific data
type PromptFlowData struct {
	Phase      string `json:"phase,omitempty"`
	Goal       string `json:"goal,omitempty"`
	Template   string `json:"template,omitempty"`
	MetaPrompt string `json:"meta_prompt,omitempty"`
	TaskPrompt string `json:"task_prompt,omitempty"`
}

// CommitGenData holds commit generation specific data
type CommitGenData struct {
	Subject     string `json:"subject,omitempty"`
	Body        string `json:"body,omitempty"`
	Type        string `json:"type,omitempty"`        // feat, fix, docs, etc.
	Scope       string `json:"scope,omitempty"`       // optional scope
	Breaking    bool   `json:"breaking,omitempty"`    // breaking change flag
	Footer      string `json:"footer,omitempty"`      // optional footer
	Template    string `json:"template,omitempty"`    // template used
	GeneratedAt time.Time `json:"generated_at,omitempty"`
}

// InteropManager manages shared context between modes
type InteropManager struct {
	ctx       context.Context
	cachePath string
}

// NewInteropManager creates a new interop manager
func NewInteropManager(ctx context.Context) *InteropManager {
	// Use current directory for cache by default
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	cachePath := filepath.Join(cwd, ".osm-interop.json")

	return &InteropManager{
		ctx:       ctx,
		cachePath: cachePath,
	}
}

// SaveSharedContext saves shared context to disk
func (im *InteropManager) SaveSharedContext(ctx *SharedContext) error {
	ctx.UpdatedAt = time.Now()
	if ctx.Version == "" {
		ctx.Version = "1.0"
	}

	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal shared context: %w", err)
	}

	if err := os.WriteFile(im.cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write shared context to %s: %w", im.cachePath, err)
	}

	return nil
}

// LoadSharedContext loads shared context from disk
func (im *InteropManager) LoadSharedContext() (*SharedContext, error) {
	data, err := os.ReadFile(im.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return empty context if file doesn't exist
			return &SharedContext{
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
				Version:      "1.0",
				ContextItems: make([]ContextItem, 0),
			}, nil
		}
		return nil, fmt.Errorf("failed to read shared context from %s: %w", im.cachePath, err)
	}

	var ctx SharedContext
	if err := json.Unmarshal(data, &ctx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal shared context: %w", err)
	}

	return &ctx, nil
}

// DeleteSharedContext removes the shared context file
func (im *InteropManager) DeleteSharedContext() error {
	if err := os.Remove(im.cachePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete shared context file %s: %w", im.cachePath, err)
	}
	return nil
}

// HasSharedContext checks if shared context file exists
func (im *InteropManager) HasSharedContext() bool {
	_, err := os.Stat(im.cachePath)
	return err == nil
}

// GetCachePath returns the current cache file path
func (im *InteropManager) GetCachePath() string {
	return im.cachePath
}

// SetCachePath sets a custom cache file path
func (im *InteropManager) SetCachePath(path string) {
	im.cachePath = path
}

// ModuleLoader creates a JavaScript module loader for interop functionality
func ModuleLoader(ctx context.Context) require.ModuleLoader {
	return func(runtime *goja.Runtime, module *goja.Object) {
		manager := NewInteropManager(ctx)
		
		exports := module.Get("exports").(*goja.Object)
		
		// Export the manager instance
		exports.Set("createManager", func() *InteropManager {
			return NewInteropManager(ctx)
		})
		
		// Convenience functions that work with the default manager
		exports.Set("save", func(data interface{}) error {
			// Convert JavaScript object to SharedContext
			jsonStr, err := json.Marshal(data)
			if err != nil {
				return fmt.Errorf("failed to marshal context data: %w", err)
			}
			
			var sharedCtx SharedContext
			if err := json.Unmarshal(jsonStr, &sharedCtx); err != nil {
				return fmt.Errorf("failed to unmarshal context data: %w", err)
			}
			
			return manager.SaveSharedContext(&sharedCtx)
		})
		
		exports.Set("load", func() (interface{}, error) {
			return manager.LoadSharedContext()
		})
		
		exports.Set("exists", func() bool {
			return manager.HasSharedContext()
		})
		
		exports.Set("delete", func() error {
			return manager.DeleteSharedContext()
		})
		
		exports.Set("getCachePath", func() string {
			return manager.GetCachePath()
		})
		
		exports.Set("setCachePath", func(path string) {
			manager.SetCachePath(path)
		})

		// Helper functions for working with context items
		exports.Set("exportContextItems", func(items interface{}) []ContextItem {
			// Convert JavaScript array to Go slice
			itemsJSON, err := json.Marshal(items)
			if err != nil {
				return nil
			}
			
			var contextItems []ContextItem
			if err := json.Unmarshal(itemsJSON, &contextItems); err != nil {
				return nil
			}
			
			return contextItems
		})

		exports.Set("importContextItems", func(contextItems []ContextItem) interface{} {
			// Convert Go slice to JavaScript-compatible format
			result := runtime.NewArray()
			for i, item := range contextItems {
				obj := runtime.NewObject()
				obj.Set("id", item.ID)
				obj.Set("type", item.Type)
				obj.Set("label", item.Label)
				obj.Set("payload", item.Payload)
				result.Set(fmt.Sprintf("%d", i), obj)
			}
			return result
		})

		// Generate commit message from context
		exports.Set("generateCommitMessage", func(contextItems []ContextItem, options interface{}) *CommitGenData {
			return generateCommitMessage(contextItems, options)
		})
	}
}

// generateCommitMessage creates a commit message from context items
func generateCommitMessage(contextItems []ContextItem, options interface{}) *CommitGenData {
	commitData := &CommitGenData{
		GeneratedAt: time.Now(),
		Type:        "feat", // default type
	}

	// Basic analysis of context items to generate a meaningful commit message
	var hasFiles, hasDiffs, hasNotes bool
	var fileCount, diffCount, noteCount int

	for _, item := range contextItems {
		switch item.Type {
		case "file":
			hasFiles = true
			fileCount++
		case "diff", "lazy-diff":
			hasDiffs = true
			diffCount++
		case "note":
			hasNotes = true
			noteCount++
		}
	}

	// Generate subject based on context
	subject := ""
	if hasDiffs && hasFiles {
		subject = fmt.Sprintf("Update %d files with changes", fileCount)
		commitData.Type = "feat"
	} else if hasDiffs {
		subject = fmt.Sprintf("Apply %d changes", diffCount)
		commitData.Type = "fix"
	} else if hasFiles {
		subject = fmt.Sprintf("Add %d files", fileCount)
		commitData.Type = "feat"
	} else if hasNotes {
		subject = "Update documentation with notes"
		commitData.Type = "docs"
	} else {
		subject = "Implement changes from one-shot-man workflow"
		commitData.Type = "feat"
	}

	commitData.Subject = subject

	// Generate body with context summary
	body := "Generated from one-shot-man interop context:\n\n"
	if hasFiles {
		body += fmt.Sprintf("- %d files processed\n", fileCount)
	}
	if hasDiffs {
		body += fmt.Sprintf("- %d diffs applied\n", diffCount)
	}
	if hasNotes {
		body += fmt.Sprintf("- %d notes included\n", noteCount)
	}

	commitData.Body = body

	return commitData
}

// GenerateCommitMessage is the public version of generateCommitMessage for testing
func GenerateCommitMessage(contextItems []ContextItem, options interface{}) *CommitGenData {
	return generateCommitMessage(contextItems, options)
}