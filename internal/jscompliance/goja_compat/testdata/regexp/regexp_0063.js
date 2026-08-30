/*---
description: goja compat regexp 63
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 63'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 63');
