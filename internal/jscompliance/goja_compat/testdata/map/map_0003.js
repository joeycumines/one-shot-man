/*---
description: goja compat map 3
includes: [assert.js]
---*/
var m=new Map(); m.set('k',3); assert.sameValue(m.get('k'),3,'map 3');
