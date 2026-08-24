/*---
description: goja compat map 58
includes: [assert.js]
---*/
var m=new Map(); m.set('k',58); assert.sameValue(m.get('k'),58,'map 58');
