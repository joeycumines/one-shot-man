/*---
description: goja compat map 66
includes: [assert.js]
---*/
var m=new Map(); m.set('k',66); assert.sameValue(m.get('k'),66,'map 66');
