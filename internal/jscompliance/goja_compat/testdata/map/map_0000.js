/*---
description: goja compat map 0
includes: [assert.js]
---*/
var m=new Map(); m.set('k',0); assert.sameValue(m.get('k'),0,'map 0');
