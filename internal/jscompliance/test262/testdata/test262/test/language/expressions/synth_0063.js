/*---
description: Synthetic test262 case 63 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(63,63), 126, 'fn 63');
