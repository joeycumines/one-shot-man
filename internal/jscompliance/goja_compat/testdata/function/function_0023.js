/*---
description: goja compat function 23
includes: [assert.js]
---*/
function f(a){return a+23} assert.sameValue(f(1), 24, 'fn 23');
