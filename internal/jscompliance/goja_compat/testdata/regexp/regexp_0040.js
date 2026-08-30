/*---
description: goja compat regexp 40
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 40'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 40');
