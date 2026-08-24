package jscompliance

import (
	"context"
	"testing"
)

func TestAdapterSurface(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/adapter_surface.spec.js", specTimeout)
}
