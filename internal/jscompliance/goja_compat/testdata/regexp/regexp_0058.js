/*---
description: goja compat regexp 58
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 58'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 58');
