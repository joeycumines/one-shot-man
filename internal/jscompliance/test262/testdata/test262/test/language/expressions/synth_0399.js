/*---
description: Synthetic test262 case 399 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(399,399), 798, 'fn 399');
