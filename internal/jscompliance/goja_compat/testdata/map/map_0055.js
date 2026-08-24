/*---
description: goja compat map 55
includes: [assert.js]
---*/
var m=new Map(); m.set('k',55); assert.sameValue(m.get('k'),55,'map 55');
