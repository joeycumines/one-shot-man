/*---
description: goja compat function 54
includes: [assert.js]
---*/
function f(a){return a+54} assert.sameValue(f(1), 55, 'fn 54');
