/*---
description: goja compat function 31
includes: [assert.js]
---*/
function f(a){return a+31} assert.sameValue(f(1), 32, 'fn 31');
