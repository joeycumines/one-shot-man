/*---
description: goja compat regexp 7
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 7'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 7');
