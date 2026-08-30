/*---
description: goja compat regexp 64
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 64'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 64');
