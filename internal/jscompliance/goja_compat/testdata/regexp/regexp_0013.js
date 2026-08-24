/*---
description: goja compat regexp 13
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 13'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 13');
