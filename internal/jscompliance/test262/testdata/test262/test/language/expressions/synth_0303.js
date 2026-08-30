/*---
description: Synthetic test262 case 303 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(303,303), 606, 'fn 303');
