exhaustive-review/phase-3: code quality and correctness fixes

All code quality fixes implemented: H2 (float formatting), H3 (nil node validation).
H1 and H5 resolved as intentional design features (Goja pattern, intentional stderr).
All tests passing with 2/2 perfect peer reviews.

Phase 3 Code Quality Improvements:
- H2 FIX: Float key formatting %f→%g in pabt/state.go (cleaner "3.14" vs "1.500000")
- H3 FIX: Nil node validation with panic in pabt/actions.go (defensive programming)
- H1 RESOLVED: Panic-based API is correct Goja pattern for JS-native functions
- H5 RESOLVED: Stderr logging intentional for critical error visibility in tests
- Test coverage: Added TestNewAction_NilNodePanic
- Documentation: CODE_QUALITY_ISSUES.md updated with all resolutions

Verification:
- Review #1: PASS PERFECT (all fixes verified correctly)
- Review #2: PASS PERFECT (all components correct, all tests passing)
- Full test suite: PASS (100%)
- Critical modules: PASS

This completes the code quality and correctness phase, with all
HIGH priority issues either fixed or properly resolved as design features.
