/*---
description: Synthetic test262 case 783 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(783,783), 1566, 'fn 783');
