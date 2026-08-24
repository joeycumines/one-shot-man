/*---
description: goja compat function 43
includes: [assert.js]
---*/
function f(a){return a+43} assert.sameValue(f(1), 44, 'fn 43');
