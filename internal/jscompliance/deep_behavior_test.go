package jscompliance

import (
	"context"
	"testing"
)

// TestModules_DeepBehavior runs the deep behavioral spec
// (pabt/bubbletea/lipgloss/unicodetext/text-template).
func TestModules_DeepBehavior(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/modules_deep.spec.js", specTimeout)
}
