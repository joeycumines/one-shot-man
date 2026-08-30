/*---
description: goja compat function 13
includes: [assert.js]
---*/
function f(a){return a+13} assert.sameValue(f(1), 14, 'fn 13');
