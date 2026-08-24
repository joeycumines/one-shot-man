/*---
description: Synthetic test262 case 599 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(599,599), 1198, 'fn 599');
