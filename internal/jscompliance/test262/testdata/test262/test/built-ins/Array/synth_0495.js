/*---
description: Synthetic test262 case 495 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(495,495), 990, 'fn 495');
