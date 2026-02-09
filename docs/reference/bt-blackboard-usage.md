# Behavior Tree Blackboard Usage

This document provides quick reference for behavior tree blackboard usage patterns.

## Quick Reference

For comprehensive documentation including blackboard usage patterns, examples, and advanced patterns, see:

**[Planning and Acting using Behavior Trees](planning-and-acting-using-behavior-trees.md)**

## Key Concepts

- **Blackboard**: A thread-safe key-value store used by behavior tree nodes to share state
- **Keys**: String identifiers for values stored in the blackboard
- **Values**: Primitive types (numbers, strings, booleans) that can be read/written by nodes

## Basic Usage

```javascript
const bt = require('osm:bt');

// Create a new blackboard
const bb = new bt.Blackboard();

// Set values
bb.set('actorX', 100);
bb.set('actorY', 200);
bb.set('isAlive', true);

// Get values
const x = bb.get('actorX');
const y = bb.get('actorY');
const alive = bb.get('isAlive');
```

## Thread Safety

The blackboard implementation uses mutex protection for concurrent access, making it safe to use from multiple goroutines or JavaScript execution contexts.

## See Also

- [osm:bt Module Documentation](../scripting.md#osmbt-behavior-trees)
- [osm:pabt Module Reference](planning-and-acting-using-behavior-trees.md)
