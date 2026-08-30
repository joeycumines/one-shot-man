/*---
description: goja compat regexp 24
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 24'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 24');
