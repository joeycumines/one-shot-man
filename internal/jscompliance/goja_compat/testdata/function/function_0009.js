/*---
description: goja compat function 9
includes: [assert.js]
---*/
function f(a){return a+9} assert.sameValue(f(1), 10, 'fn 9');
