/*---
description: goja compat function 62
includes: [assert.js]
---*/
function f(a){return a+62} assert.sameValue(f(1), 63, 'fn 62');
