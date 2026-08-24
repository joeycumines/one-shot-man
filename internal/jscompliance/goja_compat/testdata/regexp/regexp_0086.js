/*---
description: goja compat regexp 86
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 86'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 86');
