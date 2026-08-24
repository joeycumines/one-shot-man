/*---
description: goja compat regexp 79
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 79'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 79');
