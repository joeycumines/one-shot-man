/*---
description: goja compat map 19
includes: [assert.js]
---*/
var m=new Map(); m.set('k',19); assert.sameValue(m.get('k'),19,'map 19');
