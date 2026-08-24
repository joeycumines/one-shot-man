/*---
description: goja compat map 73
includes: [assert.js]
---*/
var m=new Map(); m.set('k',73); assert.sameValue(m.get('k'),73,'map 73');
