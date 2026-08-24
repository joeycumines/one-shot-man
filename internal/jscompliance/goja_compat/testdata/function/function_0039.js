/*---
description: goja compat function 39
includes: [assert.js]
---*/
function f(a){return a+39} assert.sameValue(f(1), 40, 'fn 39');
