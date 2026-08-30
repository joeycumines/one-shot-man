/*---
description: goja compat regexp 71
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 71'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 71');
