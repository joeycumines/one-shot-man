/*---
description: goja compat regexp 53
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 53'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 53');
