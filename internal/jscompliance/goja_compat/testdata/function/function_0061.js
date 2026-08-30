/*---
description: goja compat function 61
includes: [assert.js]
---*/
function f(a){return a+61} assert.sameValue(f(1), 62, 'fn 61');
