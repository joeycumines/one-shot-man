feat: Add comprehensive PABT and testing infrastructure

Major new features:
- Add PABT module (Planning and Behavior Tree) with full test coverage
- Add mouseharness testing infrastructure for TUI applications
- Comprehensive integration tests for Pick & Place workflow
- Extensive BubbleTea runtime improvements with new tests
- Enhanced scripting engine documentation and API

Review verification:
- All 8 groups (G1-G8) completed full review cycles
- All reviews passed with zero remaining issues
- Final build verification: make-all-with-log PASSED
- Total: 96 files changed, 23,751 additions, 2,747 deletions

Groups reviewed:
G1: PABT Module - Complete with expr integration tests
G2: BubbleTea - Enhanced runtime and tests
G3: Mouse Harness - New testing infrastructure
G4: Pick & Place - 6,200+ lines of comprehensive tests
G5: Shooter Game - E2E harness improvements
G6: Scripting Engine - API documentation and validation
G7: Documentation - Architecture, references, VHS tapes
G8: Config & Build - Module dependencies and build config

This work represents a 4-hour development session with rigorous
three-phase review (Review Fix Re-Review) for each component
