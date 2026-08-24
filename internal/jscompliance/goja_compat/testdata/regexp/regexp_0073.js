/*---
description: goja compat regexp 73
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 73'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 73');
