/*---
description: Synthetic test262 case 375 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(375,375), 750, 'fn 375');
