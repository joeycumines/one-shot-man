/*---
description: goja compat map 31
includes: [assert.js]
---*/
var m=new Map(); m.set('k',31); assert.sameValue(m.get('k'),31,'map 31');
