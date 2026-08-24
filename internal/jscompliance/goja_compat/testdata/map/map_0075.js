/*---
description: goja compat map 75
includes: [assert.js]
---*/
var m=new Map(); m.set('k',75); assert.sameValue(m.get('k'),75,'map 75');
