# Code Reviews

This directory contains exhaustive code review documents generated during the review and refinement process.

## Review Protocol

Each logical grouping of changes requires THREE tasks:
1. **Review** - Subagent executes with GUARANTEE correctness prompt, writes findings here
2. **Fix** - All issues addressed based on review findings
3. **Re-Review** - Second verification; if issues remain, cycle repeats

## Naming Convention

`<SEQUENCE>-<IDENTIFIER>.md`

- **SEQUENCE**: Zero-padded three-digit number (001, 002, etc.)
- **IDENTIFIER**: Short kebab-case description

## Review Status

Reviews are tracked in `/blueprint.json`.
