/*---
description: Synthetic test262 case 999 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(999,999), 1998, 'fn 999');
