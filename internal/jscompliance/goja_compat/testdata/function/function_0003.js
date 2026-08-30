/*---
description: goja compat function 3
includes: [assert.js]
---*/
function f(a){return a+3} assert.sameValue(f(1), 4, 'fn 3');
