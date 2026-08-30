/*---
description: goja compat function 10
includes: [assert.js]
---*/
function f(a){return a+10} assert.sameValue(f(1), 11, 'fn 10');
