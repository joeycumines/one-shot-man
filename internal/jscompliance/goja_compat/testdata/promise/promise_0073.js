/*---
description: goja compat promise 73
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(73) instanceof Promise, true, 'promise instance 73'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 73');
