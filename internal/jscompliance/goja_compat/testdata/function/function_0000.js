/*---
description: goja compat function 0
includes: [assert.js]
---*/
function f(a){return a+0} assert.sameValue(f(1), 1, 'fn 0');
