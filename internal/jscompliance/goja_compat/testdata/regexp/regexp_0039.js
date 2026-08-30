/*---
description: goja compat regexp 39
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 39'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 39');
