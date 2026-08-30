/*---
description: Synthetic test262 case 511 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(511,511), 1022, 'fn 511');
