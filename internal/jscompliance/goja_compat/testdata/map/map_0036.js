/*---
description: goja compat map 36
includes: [assert.js]
---*/
var m=new Map(); m.set('k',36); assert.sameValue(m.get('k'),36,'map 36');
