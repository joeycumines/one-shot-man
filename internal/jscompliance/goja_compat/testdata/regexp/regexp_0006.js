/*---
description: goja compat regexp 6
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 6'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 6');
