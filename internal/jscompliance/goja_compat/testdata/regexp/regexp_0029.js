/*---
description: goja compat regexp 29
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 29'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 29');
