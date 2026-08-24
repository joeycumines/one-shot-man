/*---
description: goja compat regexp 97
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 97'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 97');
