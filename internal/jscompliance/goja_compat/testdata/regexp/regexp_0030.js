/*---
description: goja compat regexp 30
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 30'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 30');
