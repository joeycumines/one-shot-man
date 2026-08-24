/*---
description: goja compat regexp 42
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 42'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 42');
