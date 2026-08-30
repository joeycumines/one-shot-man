# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.0] - 2026-02-10

### Added
- PA-BT (Planning-Augmented Behavior Trees) module for autonomous agent behaviors with planning capabilities
- `NewAction`, `NewActionGenerator`, `NewBlackboard`, `NewExprCondition` APIs for behavior tree planning
- `scripts/example-05-pick-and-place.js` demonstrating PA-BT for pick-and-place tasks
- `QueueGetGlobal(name, callback)` for thread-safe asynchronous global reads from scripting engine
- PA-BT documentation: API reference, demo script guide, blackboard usage guide
- Edge case test suites for commands, sessions, and platform-specific scenarios
- Performance benchmarks and regression tests
- Security test suite: 42 subtests covering path traversal, command injection, env sanitization, permissions, input validation, session isolation, and output sanitization

### Fixed
- Race condition in scripting engine: `GetGlobal()` now uses full `Lock()` for synchronization with `QueueSetGlobal()`
- Symlink vulnerability in config loading: `os.Lstat()` check rejects symlinks before opening config files

### Security
- Config file loading rejects symlinks to prevent path traversal attacks
