/*---
description: goja compat function 52
includes: [assert.js]
---*/
function f(a){return a+52} assert.sameValue(f(1), 53, 'fn 52');
