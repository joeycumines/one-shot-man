/*---
description: goja compat regexp 75
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 75'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 75');
