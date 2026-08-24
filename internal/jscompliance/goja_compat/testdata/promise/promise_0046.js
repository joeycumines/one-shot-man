/*---
description: goja compat promise 46
includes: [assert.js]
---*/
assert.sameValue(Promise.resolve(46) instanceof Promise, true, 'promise instance 46'); assert.sameValue(typeof Promise.resolve, 'function', 'promise resolve fn 46');
