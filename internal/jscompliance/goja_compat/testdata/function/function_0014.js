/*---
description: goja compat function 14
includes: [assert.js]
---*/
function f(a){return a+14} assert.sameValue(f(1), 15, 'fn 14');
