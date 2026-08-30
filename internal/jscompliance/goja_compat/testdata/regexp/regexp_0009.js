/*---
description: goja compat regexp 9
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 9'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 9');
