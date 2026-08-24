/*---
description: Synthetic test262 case 183 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(183,183), 366, 'fn 183');
