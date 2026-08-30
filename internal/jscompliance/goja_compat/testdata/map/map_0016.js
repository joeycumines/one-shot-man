/*---
description: goja compat map 16
includes: [assert.js]
---*/
var m=new Map(); m.set('k',16); assert.sameValue(m.get('k'),16,'map 16');
