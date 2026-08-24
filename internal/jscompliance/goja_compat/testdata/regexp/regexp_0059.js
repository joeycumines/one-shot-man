/*---
description: goja compat regexp 59
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 59'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 59');
