/*---
description: goja compat map 21
includes: [assert.js]
---*/
var m=new Map(); m.set('k',21); assert.sameValue(m.get('k'),21,'map 21');
