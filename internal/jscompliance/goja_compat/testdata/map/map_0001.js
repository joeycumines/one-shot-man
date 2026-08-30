/*---
description: goja compat map 1
includes: [assert.js]
---*/
var m=new Map(); m.set('k',1); assert.sameValue(m.get('k'),1,'map 1');
