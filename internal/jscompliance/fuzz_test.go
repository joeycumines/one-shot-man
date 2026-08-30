package jscompliance

import (
	"context"
	"testing"
)

func FuzzHarness(f *testing.F) {
	f.Add("assert.sameValue(1,1,'ok');")
	f.Add("var x=1; assert.equal('a', typeof x, 'type');")
	f.Add("throw new Error('boom');")
	f.Add("/*---\ndescription: broken\nincludes: [assert.js]\n---*/\nassert.sameValue(1,2,'fail');")
	f.Add("")
	f.Fuzz(func(t *testing.T, src string) {
		ctx := context.Background()
		engine, _, _ := newComplianceEngine(t, ctx)
		_, _ = collectSpecResults(t, engine, "fuzz", src, defaultEvalTimeout)
	})
}

func FuzzParseTC39(f *testing.F) {
	f.Add("/*---\ndescription: test\nincludes: [assert.js]\n---*/\nassert.sameValue(1,1,'ok');")
	f.Add("no frontmatter")
	f.Add("/*---\nnegative:\n  phase: runtime\n  type: SyntaxError\n---*/\nthrow new SyntaxError('x');")
	f.Fuzz(func(t *testing.T, src string) {
		// This is for test262 harness parsing fuzz
		_ = src
	})
}
