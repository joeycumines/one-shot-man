/*---
description: Synthetic test262 case 655 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(655,655), 1310, 'fn 655');
