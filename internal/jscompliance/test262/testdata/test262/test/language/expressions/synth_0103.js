/*---
description: Synthetic test262 case 103 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(103,103), 206, 'fn 103');
