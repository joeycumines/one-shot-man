/*---
description: goja compat function 32
includes: [assert.js]
---*/
function f(a){return a+32} assert.sameValue(f(1), 33, 'fn 32');
