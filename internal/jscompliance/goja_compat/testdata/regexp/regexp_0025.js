/*---
description: goja compat regexp 25
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 25'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 25');
