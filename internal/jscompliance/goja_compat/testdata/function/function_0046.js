/*---
description: goja compat function 46
includes: [assert.js]
---*/
function f(a){return a+46} assert.sameValue(f(1), 47, 'fn 46');
