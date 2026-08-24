/*---
description: goja compat regexp 34
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 34'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 34');
