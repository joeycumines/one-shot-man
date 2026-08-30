/*---
description: Synthetic test262 case 543 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(543,543), 1086, 'fn 543');
