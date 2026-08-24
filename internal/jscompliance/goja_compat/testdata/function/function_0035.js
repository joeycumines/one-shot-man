/*---
description: goja compat function 35
includes: [assert.js]
---*/
function f(a){return a+35} assert.sameValue(f(1), 36, 'fn 35');
