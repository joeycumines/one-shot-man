/*---
description: goja compat function 21
includes: [assert.js]
---*/
function f(a){return a+21} assert.sameValue(f(1), 22, 'fn 21');
