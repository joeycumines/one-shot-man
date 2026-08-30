/*---
description: goja compat regexp 92
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 92'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 92');
