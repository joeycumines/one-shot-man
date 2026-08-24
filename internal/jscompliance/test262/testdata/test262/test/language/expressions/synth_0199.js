/*---
description: Synthetic test262 case 199 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(199,199), 398, 'fn 199');
