/*---
description: goja compat regexp 96
includes: [assert.js]
---*/
assert.sameValue(/a/.test('a'), true, 'regexp 96'); assert.sameValue('hello'.replace(/l/g,'L'), 'heLLo', 'replace 96');
