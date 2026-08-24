/*---
description: goja compat map 17
includes: [assert.js]
---*/
var m=new Map(); m.set('k',17); assert.sameValue(m.get('k'),17,'map 17');
