/*---
description: goja compat regexp 82
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 82'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 82');
