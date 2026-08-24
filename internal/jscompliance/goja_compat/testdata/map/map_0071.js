/*---
description: goja compat map 71
includes: [assert.js]
---*/
var m=new Map(); m.set('k',71); assert.sameValue(m.get('k'),71,'map 71');
