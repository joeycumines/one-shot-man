/*---
description: goja compat regexp 1
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 1'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 1');
