/*---
description: goja compat regexp 80
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 80'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 80');
