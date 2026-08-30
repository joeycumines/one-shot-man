/*---
description: goja compat regexp 68
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 68'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 68');
