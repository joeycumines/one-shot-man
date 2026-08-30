/*---
description: goja compat regexp 43
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 43'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 43');
