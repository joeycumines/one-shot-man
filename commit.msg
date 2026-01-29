docs: standardize API naming conventions and add bt.tick() docs

pabt.md:
- Standardize on lowercase node() method (Node() is deprecated)
- Recommend lowercase value in effect objects (Value is legacy)
- Add note clarifying both cases are supported for backwards compat

bt-blackboard-usage.md:
- Add documentation for bt.tick(node) function
- Add brief reference for bt.newTicker(interval, node)
