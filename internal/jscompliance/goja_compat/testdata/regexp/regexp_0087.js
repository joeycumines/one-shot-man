/*---
description: goja compat regexp 87
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 87'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 87');
