/*---
description: Synthetic test262 case 111 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(111,111), 222, 'fn 111');
