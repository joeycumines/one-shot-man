/*---
description: Synthetic test262 case 135 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(135,135), 270, 'fn 135');
