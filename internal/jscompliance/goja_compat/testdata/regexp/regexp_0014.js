/*---
description: goja compat regexp 14
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 14'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 14');
