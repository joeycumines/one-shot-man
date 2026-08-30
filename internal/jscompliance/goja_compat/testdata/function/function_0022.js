/*---
description: goja compat function 22
includes: [assert.js]
---*/
function f(a){return a+22} assert.sameValue(f(1), 23, 'fn 22');
