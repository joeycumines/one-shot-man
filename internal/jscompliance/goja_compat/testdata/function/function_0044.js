/*---
description: goja compat function 44
includes: [assert.js]
---*/
function f(a){return a+44} assert.sameValue(f(1), 45, 'fn 44');
