/*---
description: goja compat function 68
includes: [assert.js]
---*/
function f(a){return a+68} assert.sameValue(f(1), 69, 'fn 68');
