/*---
description: goja compat function 28
includes: [assert.js]
---*/
function f(a){return a+28} assert.sameValue(f(1), 29, 'fn 28');
