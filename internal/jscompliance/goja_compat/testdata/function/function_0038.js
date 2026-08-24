/*---
description: goja compat function 38
includes: [assert.js]
---*/
function f(a){return a+38} assert.sameValue(f(1), 39, 'fn 38');
