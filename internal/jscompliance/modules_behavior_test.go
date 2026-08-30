package jscompliance

import (
	"context"
	"testing"
)

// TestModules_PureBehavior runs the pure-module behavioral VALUE spec
// (crypto/encoding/json/regexp/format/argv/flag) — the gut-check coverage.
func TestModules_PureBehavior(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/modules_pure.spec.js", specTimeout)
}
