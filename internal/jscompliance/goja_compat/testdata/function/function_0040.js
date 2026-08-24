/*---
description: goja compat function 40
includes: [assert.js]
---*/
function f(a){return a+40} assert.sameValue(f(1), 41, 'fn 40');
