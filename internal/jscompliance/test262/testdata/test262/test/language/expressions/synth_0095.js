/*---
description: Synthetic test262 case 95 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(95,95), 190, 'fn 95');
