/*---
description: goja compat function 64
includes: [assert.js]
---*/
function f(a){return a+64} assert.sameValue(f(1), 65, 'fn 64');
