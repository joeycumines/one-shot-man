/*---
description: goja compat regexp 15
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 15'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 15');
