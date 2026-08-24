/*---
description: goja compat function 36
includes: [assert.js]
---*/
function f(a){return a+36} assert.sameValue(f(1), 37, 'fn 36');
