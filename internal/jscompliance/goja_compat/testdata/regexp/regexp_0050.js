/*---
description: goja compat regexp 50
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 50'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 50');
