/*---
description: goja compat regexp 41
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 41'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 41');
