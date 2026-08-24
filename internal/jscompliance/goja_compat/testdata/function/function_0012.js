/*---
description: goja compat function 12
includes: [assert.js]
---*/
function f(a){return a+12} assert.sameValue(f(1), 13, 'fn 12');
