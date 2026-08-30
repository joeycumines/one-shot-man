/*---
description: Synthetic test262 case 775 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(775,775), 1550, 'fn 775');
