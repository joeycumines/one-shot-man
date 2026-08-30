/*---
description: goja compat regexp 31
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 31'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 31');
