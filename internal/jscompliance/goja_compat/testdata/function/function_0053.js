/*---
description: goja compat function 53
includes: [assert.js]
---*/
function f(a){return a+53} assert.sameValue(f(1), 54, 'fn 53');
