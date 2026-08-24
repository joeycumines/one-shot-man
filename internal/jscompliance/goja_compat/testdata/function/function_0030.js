/*---
description: goja compat function 30
includes: [assert.js]
---*/
function f(a){return a+30} assert.sameValue(f(1), 31, 'fn 30');
