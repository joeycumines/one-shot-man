/*---
description: goja compat regexp 10
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 10'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 10');
