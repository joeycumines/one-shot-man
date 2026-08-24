/*---
description: goja compat function 29
includes: [assert.js]
---*/
function f(a){return a+29} assert.sameValue(f(1), 30, 'fn 29');
