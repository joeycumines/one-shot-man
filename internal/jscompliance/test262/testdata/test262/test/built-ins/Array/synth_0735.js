/*---
description: Synthetic test262 case 735 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(735,735), 1470, 'fn 735');
