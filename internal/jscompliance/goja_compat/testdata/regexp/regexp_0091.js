/*---
description: goja compat regexp 91
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 91'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 91');
