/*---
description: goja compat map 27
includes: [assert.js]
---*/
var m=new Map(); m.set('k',27); assert.sameValue(m.get('k'),27,'map 27');
