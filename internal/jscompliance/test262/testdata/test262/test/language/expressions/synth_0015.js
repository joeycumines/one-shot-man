/*---
description: Synthetic test262 case 15 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(15,15), 30, 'fn 15');
