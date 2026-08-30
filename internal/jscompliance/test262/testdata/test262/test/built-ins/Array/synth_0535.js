/*---
description: Synthetic test262 case 535 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(535,535), 1070, 'fn 535');
