/*---
description: goja compat regexp 61
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 61'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 61');
