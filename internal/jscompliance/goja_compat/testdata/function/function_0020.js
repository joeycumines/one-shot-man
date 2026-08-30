/*---
description: goja compat function 20
includes: [assert.js]
---*/
function f(a){return a+20} assert.sameValue(f(1), 21, 'fn 20');
