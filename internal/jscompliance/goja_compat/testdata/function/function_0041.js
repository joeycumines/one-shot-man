/*---
description: goja compat function 41
includes: [assert.js]
---*/
function f(a){return a+41} assert.sameValue(f(1), 42, 'fn 41');
