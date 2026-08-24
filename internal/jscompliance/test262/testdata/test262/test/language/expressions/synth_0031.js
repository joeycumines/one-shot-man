/*---
description: Synthetic test262 case 31 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(31,31), 62, 'fn 31');
