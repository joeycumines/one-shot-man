/*---
description: goja compat function 19
includes: [assert.js]
---*/
function f(a){return a+19} assert.sameValue(f(1), 20, 'fn 19');
