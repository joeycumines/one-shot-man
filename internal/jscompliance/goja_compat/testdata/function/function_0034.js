/*---
description: goja compat function 34
includes: [assert.js]
---*/
function f(a){return a+34} assert.sameValue(f(1), 35, 'fn 34');
