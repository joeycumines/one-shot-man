/*---
description: goja compat map 40
includes: [assert.js]
---*/
var m=new Map(); m.set('k',40); assert.sameValue(m.get('k'),40,'map 40');
