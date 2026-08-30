/*---
description: goja compat map 14
includes: [assert.js]
---*/
var m=new Map(); m.set('k',14); assert.sameValue(m.get('k'),14,'map 14');
