/*---
description: goja compat function 50
includes: [assert.js]
---*/
function f(a){return a+50} assert.sameValue(f(1), 51, 'fn 50');
