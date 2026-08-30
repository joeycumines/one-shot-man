/*---
description: goja compat regexp 22
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 22'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 22');
