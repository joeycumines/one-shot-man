/*---
description: goja compat regexp 45
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 45'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 45');
