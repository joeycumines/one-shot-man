/*---
description: goja compat map 72
includes: [assert.js]
---*/
var m=new Map(); m.set('k',72); assert.sameValue(m.get('k'),72,'map 72');
