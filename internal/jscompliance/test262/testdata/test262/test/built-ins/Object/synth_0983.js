/*---
description: Synthetic test262 case 983 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(983,983), 1966, 'fn 983');
