/*---
description: Synthetic test262 case 55 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(55,55), 110, 'fn 55');
