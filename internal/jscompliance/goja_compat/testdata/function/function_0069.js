/*---
description: goja compat function 69
includes: [assert.js]
---*/
function f(a){return a+69} assert.sameValue(f(1), 70, 'fn 69');
