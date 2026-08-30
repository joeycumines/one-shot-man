/*---
description: goja compat regexp 20
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 20'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 20');
