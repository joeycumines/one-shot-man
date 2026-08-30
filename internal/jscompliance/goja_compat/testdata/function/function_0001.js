/*---
description: goja compat function 1
includes: [assert.js]
---*/
function f(a){return a+1} assert.sameValue(f(1), 2, 'fn 1');
