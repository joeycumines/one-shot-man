/*---
description: goja compat function 17
includes: [assert.js]
---*/
function f(a){return a+17} assert.sameValue(f(1), 18, 'fn 17');
