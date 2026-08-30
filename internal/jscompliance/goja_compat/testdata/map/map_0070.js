/*---
description: goja compat map 70
includes: [assert.js]
---*/
var m=new Map(); m.set('k',70); assert.sameValue(m.get('k'),70,'map 70');
