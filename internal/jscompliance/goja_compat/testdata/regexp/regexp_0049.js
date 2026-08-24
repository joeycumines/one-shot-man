/*---
description: goja compat regexp 49
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 49'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 49');
