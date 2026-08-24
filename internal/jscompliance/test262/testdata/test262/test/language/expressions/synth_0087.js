/*---
description: Synthetic test262 case 87 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(87,87), 174, 'fn 87');
