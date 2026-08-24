/*---
description: goja compat function 67
includes: [assert.js]
---*/
function f(a){return a+67} assert.sameValue(f(1), 68, 'fn 67');
