/*---
description: goja compat regexp 8
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 8'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 8');
