/*---
description: Synthetic test262 case 335 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(335,335), 670, 'fn 335');
