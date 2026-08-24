/*---
description: goja compat regexp 77
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 77'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 77');
