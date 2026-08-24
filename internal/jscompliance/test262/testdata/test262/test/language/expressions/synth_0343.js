/*---
description: Synthetic test262 case 343 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(343,343), 686, 'fn 343');
