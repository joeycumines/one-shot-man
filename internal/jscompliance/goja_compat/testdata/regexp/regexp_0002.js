/*---
description: goja compat regexp 2
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 2'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 2');
