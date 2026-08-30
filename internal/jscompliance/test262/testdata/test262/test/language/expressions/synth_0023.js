/*---
description: Synthetic test262 case 23 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(23,23), 46, 'fn 23');
