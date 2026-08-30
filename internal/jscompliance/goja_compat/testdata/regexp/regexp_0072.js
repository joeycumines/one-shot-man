/*---
description: goja compat regexp 72
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 72'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 72');
