/*---
description: goja compat function 11
includes: [assert.js]
---*/
function f(a){return a+11} assert.sameValue(f(1), 12, 'fn 11');
