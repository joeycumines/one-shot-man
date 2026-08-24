/*---
description: goja compat promise 74
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(74) instanceof Promise, true, 'promise instance 74'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 74');
