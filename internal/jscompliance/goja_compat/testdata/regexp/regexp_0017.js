/*---
description: goja compat regexp 17
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 17'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 17');
