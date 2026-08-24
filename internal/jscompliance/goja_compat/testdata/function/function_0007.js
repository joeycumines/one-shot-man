/*---
description: goja compat function 7
includes: [assert.js]
---*/
function f(a){return a+7} assert.sameValue(f(1), 8, 'fn 7');
