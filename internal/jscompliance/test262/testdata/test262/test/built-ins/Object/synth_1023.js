/*---
description: Synthetic test262 case 1023 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(1023,1023), 2046, 'fn 1023');
