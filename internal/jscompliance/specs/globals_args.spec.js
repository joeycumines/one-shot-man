// globals_args.spec.js — the `args` global.
// In the compliance harness, args is bound to [] (mirroring production's
// inline/empty-args path; DRIFT-3 documents that production binds args at the
// command layer). File-script population is exercised in resolution_test.go.

test('args is an array', function () {
	assert.equal('args is array', Array.isArray(args), true);
});

test('args is empty in the harness (inline parity)', function () {
	assert.equal('args length', args.length, 0);
});
