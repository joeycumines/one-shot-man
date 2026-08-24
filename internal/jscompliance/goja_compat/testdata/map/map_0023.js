/*---
description: goja compat map 23
includes: [assert.js]
---*/
var m=new Map(); m.set('k',23); assert.sameValue(m.get('k'),23,'map 23');
