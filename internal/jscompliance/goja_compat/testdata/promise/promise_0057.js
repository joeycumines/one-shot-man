/*---
description: goja compat promise 57
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(57) instanceof Promise, true, 'promise instance 57'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 57');
