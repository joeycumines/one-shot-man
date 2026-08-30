/*---
description: goja compat function 5
includes: [assert.js]
---*/
function f(a){return a+5} assert.sameValue(f(1), 6, 'fn 5');
