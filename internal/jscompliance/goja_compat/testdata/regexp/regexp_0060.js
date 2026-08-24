/*---
description: goja compat regexp 60
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 60'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 60');
