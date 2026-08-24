/*---
description: goja compat regexp 67
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 67'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 67');
