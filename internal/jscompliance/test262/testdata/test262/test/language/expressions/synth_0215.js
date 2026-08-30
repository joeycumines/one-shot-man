/*---
description: Synthetic test262 case 215 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(215,215), 430, 'fn 215');
