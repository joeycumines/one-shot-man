/*---
description: goja compat function 6
includes: [assert.js]
---*/
function f(a){return a+6} assert.sameValue(f(1), 7, 'fn 6');
