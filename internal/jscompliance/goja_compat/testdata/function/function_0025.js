/*---
description: goja compat function 25
includes: [assert.js]
---*/
function f(a){return a+25} assert.sameValue(f(1), 26, 'fn 25');
