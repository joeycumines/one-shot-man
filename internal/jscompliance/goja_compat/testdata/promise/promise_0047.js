/*---
description: goja compat promise 47
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(47) instanceof Promise, true, 'promise instance 47'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 47');
