/*---
description: goja compat regexp 94
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 94'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 94');
