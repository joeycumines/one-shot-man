/*---
description: goja compat regexp 33
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 33'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 33');
