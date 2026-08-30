/*---
description: goja compat promise 84
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(84) instanceof Promise, true, 'promise instance 84'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 84');
