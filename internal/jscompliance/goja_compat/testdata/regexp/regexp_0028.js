/*---
description: goja compat regexp 28
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 28'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 28');
