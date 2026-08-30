/*---
description: goja compat regexp 46
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 46'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 46');
