/*---
description: goja compat regexp 98
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 98'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 98');
