/*---
description: goja compat regexp 89
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 89'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 89');
