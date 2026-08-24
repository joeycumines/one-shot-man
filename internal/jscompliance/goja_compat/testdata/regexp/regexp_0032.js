/*---
description: goja compat regexp 32
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 32'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 32');
