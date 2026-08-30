/*---
description: goja compat function 16
includes: [assert.js]
---*/
function f(a){return a+16} assert.sameValue(f(1), 17, 'fn 16');
