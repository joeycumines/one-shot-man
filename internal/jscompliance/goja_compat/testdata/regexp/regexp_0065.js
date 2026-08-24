/*---
description: goja compat regexp 65
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 65'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 65');
