/*---
description: goja compat function 8
includes: [assert.js]
---*/
function f(a){return a+8} assert.sameValue(f(1), 9, 'fn 8');
