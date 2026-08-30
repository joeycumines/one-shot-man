/*---
description: Synthetic test262 case 975 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(975,975), 1950, 'fn 975');
