/*---
description: goja compat map 45
includes: [assert.js]
---*/
var m=new Map(); m.set('k',45); assert.sameValue(m.get('k'),45,'map 45');
