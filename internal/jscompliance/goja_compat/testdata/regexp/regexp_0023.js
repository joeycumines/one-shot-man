/*---
description: goja compat regexp 23
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 23'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 23');
