#!/bin/bash
set -o pipefail

echo "=== STAGING D-H3 AND D-H4 DOCUMENTATION FILES ===" 2>&1 | fold -w 200 | tee build.log
echo "" 2>&1 | fold -w 200 | tee build.log

echo "=== Adding docs/reference/bt-blackboard-usage.md (D-H3) ===" 2>&1 | fold -w 200 | tee build.log
git add docs/reference/bt-blackboard-usage.md 2>&1 | fold -w 200 | tee build.log
echo "" 2>&1 | fold -w 200 | tee build.log

echo "=== Adding docs/reference/elm-commands-and-goja.md (D-H4) ===" 2>&1 | fold -w 200 | tee build.log
git add docs/reference/elm-commands-and-goja.md 2>&1 | fold -w 200 | tee build.log
echo "" 2>&1 | fold -w 200 | tee build.log

echo "=== VERIFIED STAGED FILES ===" 2>&1 | fold -w 200 | tee build.log
git diff --cached --name-only 2>&1 | fold -w 200 | tee build.log
echo "" 2>&1 | fold -w 200 | tee build.log

echo "=== CREATING COMMIT ===" 2>&1 | fold -w 200 | tee build.log
{
  echo "docs: fix phase-4 documentation issues - D-H3 and D-H4"
  echo ""
  echo "D-H3 (HIGH): Blackboard type guidance accuracy"
  echo "- Changed \"CRITICAL constraint\" to \"Type Recommendations\" - accepts any type"
  echo "- Updated Quick Summary to remove contradictory \"ONLY supports\" language"
  echo "- Added concrete time.Time example of cross-boundary type transformation"
  echo "- Anchored Quick Summary to detailed guidance section"
  echo ""
  echo "D-H4 (HIGH): WrapCmd access boundary clarification"
  echo "- Added ⚠️ IMPORTANT: WrapCmd is Go-internal helper, not JS-accessible"
  echo "- Clarified asymmetry: Go calls WrapCmd, JS receives wrapped results"
  echo "- Added JavaScript and Go code examples showing correct usage"
  echo ""
  echo "Verification:"
  echo "- D-H3: 3 peer reviews (PASS → issue found+fixed → confirmation PASS)"
  echo "- D-H4: 2 perfect peer reviews (PASS)"
  echo ""
  echo "Both fixes now production-ready."
  echo ""
  echo "Related to PHASE_4 documentation review."
} | git commit -F - 2>&1 | fold -w 200 | tee build.log
exit_code=${PIPESTATUS[0]}
echo "" 2>&1 | fold -w 200 | tee build.log

echo "=== VERIFYING COMMIT RESULT ===" 2>&1 | fold -w 200 | tee build.log
git log -1 --stat 2>&1 | fold -w 200 | tee build.log | tail -n 50
echo "" 2>&1 | fold -w 200 | tee build.log

echo "=== COMMIT COMPLETE (exit code: ${exit_code}) ===" 2>&1 | fold -w 200 | tee build.log
exit ${exit_code}
