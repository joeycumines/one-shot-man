/*---
description: goja compat function 42
includes: [assert.js]
---*/
function f(a){return a+42} assert.sameValue(f(1), 43, 'fn 42');
