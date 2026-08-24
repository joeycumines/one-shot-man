/*---
description: goja compat map 15
includes: [assert.js]
---*/
var m=new Map(); m.set('k',15); assert.sameValue(m.get('k'),15,'map 15');
