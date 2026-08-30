/*---
description: goja compat function 18
includes: [assert.js]
---*/
function f(a){return a+18} assert.sameValue(f(1), 19, 'fn 18');
