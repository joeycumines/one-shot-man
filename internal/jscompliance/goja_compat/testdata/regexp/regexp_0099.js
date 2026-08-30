/*---
description: goja compat regexp 99
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 99'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 99');
