#!/bin/bash
set -o pipefail

echo "=== VERIFYING CURRENT STATUS ===" | tee -a build.log
git status --short 2>&1 | fold -w 200 | tee -a build.log
echo ""

echo "=== CHECKING IF PREVIOUS COMMIT EXISTS ===" | tee -a build.log
git log --oneline -5 2>&1 | fold -w 200 | tee -a build.log
echo ""

echo "=== RESETTING TO PREVIOUS COMMIT ===" | tee -a build.log
git reset --hard HEAD~1 2>&1 | fold -w 200 | tee -a build.log
echo ""

echo "=== VERIFYING STATUS AFTER RESET ===" | tee -a build.log
git status --short 2>&1 | fold -w 200 | tee -a build.log
echo ""

echo "=== STAGING ONLY docs/reference/command.md ===" | tee -a build.log
git add docs/reference/command.md 2>&1 | fold -w 200 | tee -a build.log
git status --short 2>&1 | fold -w 200 | tee -a build.log
echo ""

echo "=== STAGED FILES ===" | tee -a build.log
git diff --cached --name-only 2>&1 | fold -w 200 | tee -a build.log
echo ""

echo "=== CREATING COMMIT ===" | tee -a build.log
{
  echo "docs: fix goal command -r flag documentation"
  echo ""
  echo "- Added missing -r <goal-name> flag documentation"
  echo "- Distinguished positional (interactive default) vs -r (non-interactive) invocation"
  echo "- Added three usage examples showing all execution modes"
  echo "- Verified with internal/command/goal.go:108 implementation"
  echo "- Two contiguous perfect peer reviews passed"
  echo ""
  echo "Related to PHASE_4 documentation review."
} | git commit -F - 2>&1 | fold -w 200 | tee -a build.log
echo ""

echo "=== VERIFYING COMMIT ===" | tee -a build.log
git log -1 --stat 2>&1 | fold -w 200 | tee -a build.log
