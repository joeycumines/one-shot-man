/*---
description: goja compat function 51
includes: [assert.js]
---*/
function f(a){return a+51} assert.sameValue(f(1), 52, 'fn 51');
