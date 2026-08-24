/*---
description: goja compat map 47
includes: [assert.js]
---*/
var m=new Map(); m.set('k',47); assert.sameValue(m.get('k'),47,'map 47');
