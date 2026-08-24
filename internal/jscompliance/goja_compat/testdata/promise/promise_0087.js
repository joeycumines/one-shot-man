/*---
description: goja compat promise 87
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(87) instanceof Promise, true, 'promise instance 87'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 87');
