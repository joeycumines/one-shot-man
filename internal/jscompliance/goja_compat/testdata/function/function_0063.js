/*---
description: goja compat function 63
includes: [assert.js]
---*/
function f(a){return a+63} assert.sameValue(f(1), 64, 'fn 63');
