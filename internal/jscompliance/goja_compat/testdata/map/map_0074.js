/*---
description: goja compat map 74
includes: [assert.js]
---*/
var m=new Map(); m.set('k',74); assert.sameValue(m.get('k'),74,'map 74');
