/*---
description: goja compat regexp 44
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 44'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 44');
