/*---
description: goja compat function 59
includes: [assert.js]
---*/
function f(a){return a+59} assert.sameValue(f(1), 60, 'fn 59');
