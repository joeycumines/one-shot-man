/*---
description: goja compat regexp 3
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 3'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 3');
