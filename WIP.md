# WIP — Session (T236)

## Current State

- **T200-T236**: Done (committed)
  - T235=74de76f, T236=pending commit
- **T237**: Next — MacOSUseSDK integration evaluation

## T236 Summary

Implemented go-prompt enhancements:
1. History persistence via saveHistory (dedup, trim, MkdirAll, 0600 perms)
2. completionWordSeparator config field
3. indentSize config field
4. promptHistoryConfigs map for tracking per-prompt history configs

Bug fixes during review gate:
- Fixed saveHistory dedup panic when first entry is empty (len(deduped)==0 vs i==0)
- Added Windows skip for file permissions test
- Added regression test for first-entry-empty scenario

Review Gate: 2 contiguous passes + fitness review passed.

## Immediate Next Step

1. Commit T236
2. Start T237 (evaluation/decision task)
