/*---
description: goja compat function 2
includes: [assert.js]
---*/
function f(a){return a+2} assert.sameValue(f(1), 3, 'fn 2');
