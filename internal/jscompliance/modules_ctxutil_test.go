package jscompliance

import (
	"context"
	"strings"
	"testing"
)

// TestSlow_Ctxutil_BuildContext pins osm:ctxutil.buildContext (async): it
// returns a Promise resolving to a rendered context string that includes the
// provided items' content.
func TestSlow_Ctxutil_BuildContext(t *testing.T) {
	skipSlow(t)
	t.Parallel()
	ctx := context.Background()
	engine, _, _ := newComplianceEngine(t, ctx)
	v, err := evalJS(t, engine, `await require('osm:ctxutil').buildContext([
		{ type: 'note', label: 'a.go', payload: 'package main' }
	])`, defaultEvalTimeout)
	if err != nil {
		t.Fatalf("buildContext failed: %v", err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("buildContext resolved to %T, want string", v)
	}
	if !strings.Contains(s, "package main") {
		t.Errorf("buildContext result missing item content; got %q", s)
	}
}
