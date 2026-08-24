/*---
description: goja compat map 48
includes: [assert.js]
---*/
var m=new Map(); m.set('k',48); assert.sameValue(m.get('k'),48,'map 48');
