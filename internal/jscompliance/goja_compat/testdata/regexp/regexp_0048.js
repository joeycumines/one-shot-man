/*---
description: goja compat regexp 48
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 48'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 48');
