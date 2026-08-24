/*---
description: goja compat function 15
includes: [assert.js]
---*/
function f(a){return a+15} assert.sameValue(f(1), 16, 'fn 15');
