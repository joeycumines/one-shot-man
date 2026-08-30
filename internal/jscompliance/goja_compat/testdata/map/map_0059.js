/*---
description: goja compat map 59
includes: [assert.js]
---*/
var m=new Map(); m.set('k',59); assert.sameValue(m.get('k'),59,'map 59');
