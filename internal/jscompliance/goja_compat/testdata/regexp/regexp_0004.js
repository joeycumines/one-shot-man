/*---
description: goja compat regexp 4
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 4'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 4');
