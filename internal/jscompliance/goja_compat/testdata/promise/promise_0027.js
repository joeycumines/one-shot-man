/*---
description: goja compat promise 27
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(27) instanceof Promise, true, 'promise instance 27'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 27');
