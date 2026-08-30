/*---
description: goja compat function 24
includes: [assert.js]
---*/
function f(a){return a+24} assert.sameValue(f(1), 25, 'fn 24');
