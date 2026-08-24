/*---
description: goja compat function 37
includes: [assert.js]
---*/
function f(a){return a+37} assert.sameValue(f(1), 38, 'fn 37');
