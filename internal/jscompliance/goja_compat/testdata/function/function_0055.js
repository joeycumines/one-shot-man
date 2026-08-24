/*---
description: goja compat function 55
includes: [assert.js]
---*/
function f(a){return a+55} assert.sameValue(f(1), 56, 'fn 55');
