/*---
description: goja compat regexp 84
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 84'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 84');
