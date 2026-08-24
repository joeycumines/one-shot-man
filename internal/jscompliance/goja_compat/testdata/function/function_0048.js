/*---
description: goja compat function 48
includes: [assert.js]
---*/
function f(a){return a+48} assert.sameValue(f(1), 49, 'fn 48');
