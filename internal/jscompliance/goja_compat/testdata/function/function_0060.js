/*---
description: goja compat function 60
includes: [assert.js]
---*/
function f(a){return a+60} assert.sameValue(f(1), 61, 'fn 60');
