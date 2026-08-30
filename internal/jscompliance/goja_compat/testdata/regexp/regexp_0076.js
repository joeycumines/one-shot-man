/*---
description: goja compat regexp 76
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 76'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 76');
