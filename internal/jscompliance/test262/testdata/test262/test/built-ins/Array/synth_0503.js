/*---
description: Synthetic test262 case 503 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(503,503), 1006, 'fn 503');
