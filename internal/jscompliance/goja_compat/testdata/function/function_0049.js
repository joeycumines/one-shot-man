/*---
description: goja compat function 49
includes: [assert.js]
---*/
function f(a){return a+49} assert.sameValue(f(1), 50, 'fn 49');
