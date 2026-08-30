/*---
description: goja compat map 79
includes: [assert.js]
---*/
var m=new Map(); m.set('k',79); assert.sameValue(m.get('k'),79,'map 79');
