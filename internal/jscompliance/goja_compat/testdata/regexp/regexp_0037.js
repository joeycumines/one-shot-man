/*---
description: goja compat regexp 37
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 37'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 37');
