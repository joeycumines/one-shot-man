/*---
description: Synthetic test262 case 415 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(415,415), 830, 'fn 415');
