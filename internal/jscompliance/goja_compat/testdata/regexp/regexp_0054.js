/*---
description: goja compat regexp 54
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 54'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 54');
