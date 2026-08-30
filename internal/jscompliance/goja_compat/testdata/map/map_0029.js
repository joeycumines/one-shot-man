/*---
description: goja compat map 29
includes: [assert.js]
---*/
var m=new Map(); m.set('k',29); assert.sameValue(m.get('k'),29,'map 29');
