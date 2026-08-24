/*---
description: goja compat regexp 70
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 70'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 70');
