/*---
description: goja compat regexp 36
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 36'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 36');
