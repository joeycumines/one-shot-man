/*---
description: goja compat regexp 55
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 55'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 55');
