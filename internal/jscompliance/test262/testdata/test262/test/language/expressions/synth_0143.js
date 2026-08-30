/*---
description: Synthetic test262 case 143 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(143,143), 286, 'fn 143');
