package jscompliance

import (
	"context"
	"testing"
)

func TestWebAPI(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	runSpec(t, engine, "specs/web_api.spec.js", specTimeout)
}
