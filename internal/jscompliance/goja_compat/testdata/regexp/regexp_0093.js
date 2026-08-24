/*---
description: goja compat regexp 93
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 93'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 93');
