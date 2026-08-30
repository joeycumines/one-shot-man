package jscompliance

import (
	"context"
	"testing"
)

// TestModules_FrameworkBehavior runs the framework-modules behavioral spec
// (bt/lipgloss/bubblezone/bubbles/termui/aimux-parser).
func TestModules_FrameworkBehavior(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/modules_framework.spec.js", specTimeout)
}
