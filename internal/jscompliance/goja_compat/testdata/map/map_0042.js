/*---
description: goja compat map 42
includes: [assert.js]
---*/
var m=new Map(); m.set('k',42); assert.sameValue(m.get('k'),42,'map 42');
