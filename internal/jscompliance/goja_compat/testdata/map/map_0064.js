/*---
description: goja compat map 64
includes: [assert.js]
---*/
var m=new Map(); m.set('k',64); assert.sameValue(m.get('k'),64,'map 64');
