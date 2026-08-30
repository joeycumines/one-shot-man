/*---
description: goja compat function 45
includes: [assert.js]
---*/
function f(a){return a+45} assert.sameValue(f(1), 46, 'fn 45');
