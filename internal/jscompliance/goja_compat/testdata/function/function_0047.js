/*---
description: goja compat function 47
includes: [assert.js]
---*/
function f(a){return a+47} assert.sameValue(f(1), 48, 'fn 47');
