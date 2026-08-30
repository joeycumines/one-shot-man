/*---
description: goja compat regexp 21
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 21'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 21');
