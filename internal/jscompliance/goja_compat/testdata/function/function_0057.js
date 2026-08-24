/*---
description: goja compat function 57
includes: [assert.js]
---*/
function f(a){return a+57} assert.sameValue(f(1), 58, 'fn 57');
