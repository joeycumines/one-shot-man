/*---
description: goja compat map 78
includes: [assert.js]
---*/
var m=new Map(); m.set('k',78); assert.sameValue(m.get('k'),78,'map 78');
