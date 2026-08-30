/*---
description: goja compat regexp 83
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 83'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 83');
