/*---
description: goja compat function 33
includes: [assert.js]
---*/
function f(a){return a+33} assert.sameValue(f(1), 34, 'fn 33');
