/*---
description: goja compat regexp 56
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 56'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 56');
