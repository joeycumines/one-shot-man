/*---
description: goja compat map 41
includes: [assert.js]
---*/
var m=new Map(); m.set('k',41); assert.sameValue(m.get('k'),41,'map 41');
