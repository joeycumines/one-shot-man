/*---
description: goja compat function 4
includes: [assert.js]
---*/
function f(a){return a+4} assert.sameValue(f(1), 5, 'fn 4');
