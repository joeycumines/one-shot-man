/*---
description: goja compat regexp 18
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 18'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 18');
