/*---
description: goja compat regexp 81
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 81'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 81');
