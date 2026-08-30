/*---
description: goja compat function 26
includes: [assert.js]
---*/
function f(a){return a+26} assert.sameValue(f(1), 27, 'fn 26');
