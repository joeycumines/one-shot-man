/*---
description: goja compat regexp 52
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 52'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 52');
