/*---
description: goja compat regexp 11
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 11'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 11');
