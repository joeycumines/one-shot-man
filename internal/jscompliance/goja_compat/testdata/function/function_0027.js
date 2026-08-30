/*---
description: goja compat function 27
includes: [assert.js]
---*/
function f(a){return a+27} assert.sameValue(f(1), 28, 'fn 27');
