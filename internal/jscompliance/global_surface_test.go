package jscompliance

import (
	"context"
	"testing"
)

// TestGlobalSurface runs the ES2020+ adapter-global inventory spec.
func TestGlobalSurface(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/global_surface.spec.js", specTimeout)
}
