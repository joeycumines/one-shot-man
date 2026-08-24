/*---
description: goja compat regexp 95
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 95'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 95');
