/*---
description: goja compat promise 91
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(91) instanceof Promise, true, 'promise instance 91'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 91');
