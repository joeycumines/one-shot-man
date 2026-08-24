/*---
description: goja compat regexp 51
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 51'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 51');
