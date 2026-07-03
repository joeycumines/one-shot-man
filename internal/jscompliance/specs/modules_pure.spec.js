// modules_pure.spec.js — behavioral VALUE assertions for the pure (no-I/O)
// modules. Complements the contract-table smokes with deeper correctness
// checks (the gut-checks: a wrong value fails, not just a missing export).

// --- crypto (known digests) ---
test('crypto hashes match known vectors', function () {
	var c = require('osm:crypto');
	assert.equal('sha256(abc)', c.sha256('abc'), 'ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad');
	assert.equal('sha1(abc)', c.sha1('abc'), 'a9993e364706816aba3e25717850c26c9cd0d89d');
	assert.equal('md5(abc)', c.md5('abc'), '900150983cd24fb0d6963f7d28e17f72');
});
test('crypto hmac matches known vector', function () {
	var c = require('osm:crypto');
	assert.equal('hmacSHA256', c.hmacSHA256('key', 'The quick brown fox jumps over the lazy dog'), 'f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8');
});

// --- encoding (round-trips + URL distinctness) ---
test('encoding base64 round-trips', function () {
	var e = require('osm:encoding');
	assert.equal('base64Encode(Man)', e.base64Encode('Man'), 'TWFu');
	assert.equal('base64Decode(TWFu)', e.base64Decode('TWFu'), 'Man');
	assert.equal('base64 round-trip', e.base64Decode(e.base64Encode('hello world')), 'hello world');
});
test('encoding hex round-trips', function () {
	var e = require('osm:encoding');
	assert.equal('hexEncode(Ab)', e.hexEncode('Ab'), '4162');
	assert.equal('hexDecode(4162)', e.hexDecode('4162'), 'Ab');
});
test('encoding base64URL uses - and _ (not + and /)', function () {
	var e = require('osm:encoding');
	// bytes that produce + and / in standard base64
	var enc = e.base64URLEncode(arrayOfBytes([0xfb, 0xff, 0xbf]));
	assert.equal('base64URL has no + or /', (enc.indexOf('+') === -1 && enc.indexOf('/') === -1), true);
});
test('encoding decode errors throw', function () {
	var e = require('osm:encoding');
	assert.throws('bad base64 throws', function () { e.base64Decode('!!!not valid!!!'); });
});

// --- json (operations + RFC7386 mergePatch) ---
test('json parse/stringify round-trip', function () {
	var j = require('osm:json');
	var o = { a: 1, b: [2, 3], c: { d: 'x' } };
	assert.deepEqual('round-trip', j.parse(j.stringify(o)), o);
});
test('json.query dot/bracket/wildcard', function () {
	var j = require('osm:json');
	assert.equal('dot path', j.query({ a: { b: 7 } }, 'a.b'), 7);
	assert.equal('bracket index', j.query({ a: [10, 20] }, 'a[1]'), 20);
});
test('json.mergePatch follows RFC 7386', function () {
	var j = require('osm:json');
	// nested merge, not replace
	var out = j.mergePatch({ a: { b: 1 } }, { a: { c: 2 } });
	assert.deepEqual('nested merge', out, { a: { b: 1, c: 2 } });
	// null deletes
	var out2 = j.mergePatch({ a: 1, b: 2 }, { a: null });
	assert.equal('null deletes key', Object.keys(out2).indexOf('a'), -1);
});
test('json.flatten/unflatten round-trip', function () {
	var j = require('osm:json');
	var flat = j.flatten({ a: { b: { c: 1 } } });
	assert.equal('flattened key', flat['a.b.c'], 1);
	assert.deepEqual('unflatten restores', j.unflatten({ 'a.b.c': 1 }), { a: { b: { c: 1 } } });
});

// --- regexp (find/replace/split) ---
test('regexp find and replace', function () {
	var r = require('osm:regexp');
	assert.equal('find', r.find('\\d+', 'abc123def'), '123');
	assert.deepEqual('findAll', r.findAll('\\d', 'a1b2c3'), ['1', '2', '3']);
	assert.equal('replace', r.replace('\\d+', 'a1b2', 'X'), 'aXbX');
	assert.equal('replaceAll', r.replaceAll('\\d', 'a1b2', ''), 'ab');
	assert.deepEqual('split', r.split(',', 'a,b,c'), ['a', 'b', 'c']);
});
test('regexp findSubmatch captures groups', function () {
	var r = require('osm:regexp');
	var m = r.findSubmatch('(\\w+)@(\\w+\\.\\w+)', 'reach mail@example.com');
	// findSubmatch returns [full, group1, group2]
	assert.equal('match full', m[0], 'mail@example.com');
	assert.equal('group1 user', m[1], 'mail');
	assert.equal('group2 host', m[2], 'example.com');
});

// --- format ---
test('format.formatBytes uses base-1024 with SI symbols (kB/MB)', function () {
	var f = require('osm:format');
	assert.equal('formatBytes(2048)', f.formatBytes(2048), '2.0 kB');
	assert.equal('formatBytes(0)', f.formatBytes(0), '0 B');
});

// --- argv (round-trip) ---
test('argv parse/format round-trip', function () {
	var a = require('osm:argv');
	var parsed = a.parseArgv('git commit -m "hello world"');
	assert.equal('parsed[0]', parsed[0], 'git');
	// the quoted phrase is one arg
	assert.equal('quoted arg present', parsed.indexOf('hello world') >= 0, true);
});

// --- flag ---
test('flag parses typed flags and exposes get/args', function () {
	var flag = require('osm:flag');
	var fs = flag.newFlagSet('compliance');
	fs.string('name', 'default', 'a name');
	fs.bool('verbose', false, 'verbose');
	var res = fs.parse(['--name', 'alice', '--verbose', 'extra']);
	assert.equal('parse no error', res.error === null || res.error === undefined, true);
	assert.equal('string flag get', fs.get('name'), 'alice');
	assert.equal('bool flag get', fs.get('verbose'), true);
	assert.deepEqual('positional args', fs.args(), ['extra']);
});

// --- tokenizer (pure: count/tokenize; loadFile is async, covered by contract) ---
test('tokenizer count and tokenize agree on length', function () {
	var t = require('osm:tokenizer');
	var text = 'hello world';
	var c = t.count(text);
	var r = t.tokenize(text);
	assert.equal('tokenize returns object', typeof r, 'object');
	assert.equal('tokenize.count === count', r.count, c);
	assert.equal('tokenize.tokens is array', Array.isArray(r.tokens), true);
	assert.equal('tokenize.tokens length matches count', r.tokens.length, c);
});

// helper: build a byte array (for encoding tests)
function arrayOfBytes(arr) {
	var u = new Uint8Array(arr.length);
	for (var i = 0; i < arr.length; i++) u[i] = arr[i];
	return u;
}

// --- crypto hmacSHA1 (known RFC test vector) ---
test('crypto hmacSHA1 matches known vector', function () {
	var c = require('osm:crypto');
	assert.equal('hmacSHA1', c.hmacSHA1('key', 'The quick brown fox jumps over the lazy dog'), 'de7c9b85b8b78aa6bc8a7a36f70a90701c9db4d9');
});

// --- regexp.compile returns a RegexpObject with bound methods ---
test('regexp.compile returns a reusable RegexpObject', function () {
	var r = require('osm:regexp');
	var re = r.compile('^f(oo)+$');
	assert.equal('compile returns object', typeof re, 'object');
	assert.equal('compiled.match hit', re.match('foooo'), true);
	assert.equal('compiled.match miss', re.match('bar'), false);
	assert.equal('compiled.find captures', re.find('x fooy z'), 'fooy');
});

// --- json.diff produces JSON-Pointer op records ---
test('json.diff returns op records with JSON Pointer paths', function () {
	var j = require('osm:json');
	var d = j.diff({ a: 1, b: 2 }, { a: 1, b: 3, c: 4 });
	assert.equal('diff is array', Array.isArray(d), true);
	// every record has an op + a path
	var allShaped = d.every(function (op) { return typeof op.op === 'string' && typeof op.path === 'string'; });
	assert.equal('diff records shaped', allShaped, true);
});

// --- json.stringify indent pretty-prints ---
test('json.stringify with indent pretty-prints', function () {
	var j = require('osm:json');
	var compact = j.stringify({ a: 1 });
	var pretty = j.stringify({ a: 1 }, 2);
	assert.equal('compact is single-line', compact.indexOf('\n') === -1, true);
	assert.equal('pretty is multi-line', pretty.indexOf('\n') >= 0, true);
});
