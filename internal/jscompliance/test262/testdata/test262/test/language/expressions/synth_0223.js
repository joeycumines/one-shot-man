/*---
description: Synthetic test262 case 223 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(223,223), 446, 'fn 223');
