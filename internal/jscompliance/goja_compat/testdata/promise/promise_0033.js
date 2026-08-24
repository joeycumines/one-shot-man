/*---
description: goja compat promise 33
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(33) instanceof Promise, true, 'promise instance 33'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 33');
