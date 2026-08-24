/*---
description: goja compat function 65
includes: [assert.js]
---*/
function f(a){return a+65} assert.sameValue(f(1), 66, 'fn 65');
