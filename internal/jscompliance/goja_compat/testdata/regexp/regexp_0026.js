/*---
description: goja compat regexp 26
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 26'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 26');
