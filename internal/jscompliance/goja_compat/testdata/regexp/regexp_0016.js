/*---
description: goja compat regexp 16
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 16'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 16');
