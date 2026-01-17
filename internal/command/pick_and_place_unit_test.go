package command

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
)

// ScriptContent contains inline pick-and-place utility functions
const ScriptContent = `
	// Euclidean distance calculation
	function distance(x1, y1, x2, y2) {
		return Math.sqrt(Math.pow(x2 - x1, 2) + Math.pow(y2 - y1, 2));
	}

	// Clamp value to range [min, max]
	function clamp(value, min, max) {
		return Math.max(min, Math.min(max, value));
	}
`

// TestPickAndPlace_Distance tests Euclidean distance calculation utility function
func TestPickAndPlace_Distance(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("pick-and-place", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Load inline script content
	script := engine.LoadScriptFromString("pick-utils", ScriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load utilities: %v", err)
	}

	// Test distance calculation with various inputs
	testCases := []struct {
		name     string
		x1, y1   float64
		x2, y2   float64
		expected float64
	}{
		{"Same point", 0, 0, 0, 0, 0},
		{"Horizontal line", 0, 0, 5, 0, 5},
		{"Vertical line", 0, 0, 0, 5, 5},
		{"Diagonal", 0, 0, 3, 4, 5},
		{"Negative coordinates", -1, -1, 1, 2, 3.605551275463989},
		{"Large distance", 0, 0, 100, 200, 223.60679774997897},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsCall := fmt.Sprintf("(() => { lastResult = distance(%v, %v, %v, %v); })()", tc.x1, tc.y1, tc.x2, tc.y2)
			script := engine.LoadScriptFromString("distance-call", jsCall)
			if err := engine.ExecuteScript(script); err != nil {
				t.Fatalf("Failed to calculate distance: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			if result == nil {
				t.Fatalf("Failed to retrieve distance result")
			}

			var resultFloat float64
			switch v := result.(type) {
			case float64:
				resultFloat = v
			case int64:
				resultFloat = float64(v)
			default:
				t.Fatalf("Expected float64 or int64, got %T", result)
			}

			epsilon := 1e-9
			if math.Abs(resultFloat-tc.expected) > epsilon {
				t.Errorf("Expected distance %v, got %v", tc.expected, resultFloat)
			}
		})
	}
}

// TestPickAndPlace_Clamp tests clamp utility function
func TestPickAndPlace_Clamp(t *testing.T) {
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngineWithConfig(ctx, &stdout, &stderr, testutil.NewTestSessionID("pick-and-place", t.Name()), "memory")
	if err != nil {
		t.Fatalf("NewEngineWithConfig failed: %v", err)
	}
	defer engine.Close()
	engine.SetTestMode(true)

	// Load inline script content
	script := engine.LoadScriptFromString("pick-utils", ScriptContent)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("Failed to load utilities: %v", err)
	}

	// Test clamp function with various inputs
	testCases := []struct {
		name     string
		value    float64
		min      float64
		max      float64
		expected float64
	}{
		{"Within range", 5, 0, 10, 5},
		{"Below minimum", -5, 0, 10, 0},
		{"Above maximum", 15, 0, 10, 10},
		{"At minimum", 0, 0, 10, 0},
		{"At maximum", 10, 0, 10, 10},
		{"Negative range", -5, -10, -1, -5},
		{"Below negative min", -15, -10, -1, -10},
		{"Above negative max", 5, -10, -1, -1},
		{"Zero value", 0, -5, 5, 0},
		{"Zero range", 5, 0, 0, 0},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			jsCall := fmt.Sprintf("(() => { lastResult = clamp(%v, %v, %v); })()", tc.value, tc.min, tc.max)
			script := engine.LoadScriptFromString("clamp-call", jsCall)
			if err := engine.ExecuteScript(script); err != nil {
				t.Fatalf("Failed to clamp value: %v", err)
			}

			result := engine.GetGlobal("lastResult")
			if result == nil {
				t.Fatalf("Failed to get clamp result")
			}

			var resultFloat float64
			switch v := result.(type) {
			case float64:
				resultFloat = v
			case int64:
				resultFloat = float64(v)
			default:
				t.Fatalf("Expected float64 or int64, got %T", result)
			}

			if resultFloat != tc.expected {
				t.Errorf("Expected clamp result %v, got %v", tc.expected, resultFloat)
			}
		})
	}
}
