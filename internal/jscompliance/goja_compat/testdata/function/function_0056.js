/*---
description: goja compat function 56
includes: [assert.js]
---*/
function f(a){return a+56} assert.sameValue(f(1), 57, 'fn 56');
