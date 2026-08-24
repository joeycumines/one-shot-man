/*---
description: Synthetic test262 case 423 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(423,423), 846, 'fn 423');
