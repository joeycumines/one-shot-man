/*---
description: goja compat regexp 90
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 90'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 90');
