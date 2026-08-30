/*---
description: Synthetic test262 case 567 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(567,567), 1134, 'fn 567');
