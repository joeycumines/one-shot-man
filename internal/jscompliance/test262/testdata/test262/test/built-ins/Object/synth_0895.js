/*---
description: Synthetic test262 case 895 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(895,895), 1790, 'fn 895');
