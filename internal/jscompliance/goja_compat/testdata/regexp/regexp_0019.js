/*---
description: goja compat regexp 19
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 19'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 19');
