/*---
description: goja compat map 9
includes: [assert.js]
---*/
var m=new Map(); m.set('k',9); assert.sameValue(m.get('k'),9,'map 9');
