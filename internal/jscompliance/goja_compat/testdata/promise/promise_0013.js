/*---
description: goja compat promise 13
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(13) instanceof Promise, true, 'promise instance 13'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 13');
