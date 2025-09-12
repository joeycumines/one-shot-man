# Apply this override to all Go test files.
applyTo:
  - "**/*_test.go"
---
# MANAGED ACCESS DIRECTIVE FOR TEST FILES

**CRITICAL OVERRIDE:** These files form the contract for code quality. They must be handled with deliberate care.

1.  **Justified Changes Only:** You may only modify these files if it is an explicit and justified part of your PEV plan. The reason for the modification must be clearly documented in the plan's "Justification" section.

2.  **Deception is Forbidden:** You are **STRICTLY FORBIDDEN** from making any change that reduces the effectiveness of the test suite simply to achieve a passing state. This includes commenting out tests, removing assertions, or introducing logic that forces a test to pass incorrectly.

3.  **No Configuration Changes:** You are forbidden from altering test runner configurations to exclude, skip, or otherwise ignore tests.

4.  **Invalidation Clause:** Any unplanned or unjustified modification to these files is a critical failure. You must immediately halt, revert, and report the violation as per the Circuit Breaker Protocol.
