/*---
description: goja compat regexp 74
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 74'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 74');
