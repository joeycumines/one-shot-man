/*---
description: goja compat function 66
includes: [assert.js]
---*/
function f(a){return a+66} assert.sameValue(f(1), 67, 'fn 66');
