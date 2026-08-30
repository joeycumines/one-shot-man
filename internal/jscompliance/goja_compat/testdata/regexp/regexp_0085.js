/*---
description: goja compat regexp 85
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 85'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 85');
