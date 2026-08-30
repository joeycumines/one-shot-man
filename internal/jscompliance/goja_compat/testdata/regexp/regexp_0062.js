/*---
description: goja compat regexp 62
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 62'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 62');
