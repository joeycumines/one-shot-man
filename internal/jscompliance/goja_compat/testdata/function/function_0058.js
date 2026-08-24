/*---
description: goja compat function 58
includes: [assert.js]
---*/
function f(a){return a+58} assert.sameValue(f(1), 59, 'fn 58');
