/*---
description: goja compat regexp 66
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 66'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 66');
