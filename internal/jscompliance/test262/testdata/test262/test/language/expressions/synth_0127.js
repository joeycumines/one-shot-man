/*---
description: Synthetic test262 case 127 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(127,127), 254, 'fn 127');
