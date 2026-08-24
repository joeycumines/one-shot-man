/*---
description: Synthetic test262 case 455 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(455,455), 910, 'fn 455');
