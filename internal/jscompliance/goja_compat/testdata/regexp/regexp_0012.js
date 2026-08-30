/*---
description: goja compat regexp 12
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 12'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 12');
