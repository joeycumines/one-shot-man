/*---
description: goja compat regexp 5
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 5'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 5');
