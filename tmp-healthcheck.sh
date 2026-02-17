#!/bin/bash
set -e
cd /Users/joeyc/dev/one-shot-man

echo "========== STEP 1: BUILD CHECK =========="
go build ./... 2>&1 | tail -5
echo "(build exit code: ${PIPESTATUS[0]})"

echo ""
echo "========== STEP 2: VET =========="
go vet ./... 2>&1 | tail -10
echo "(vet exit code: ${PIPESTATUS[0]})"

echo ""
echo "========== STEP 3: TESTS =========="
go test -race -count=1 -timeout=12m ./... 2>&1 | tail -30
echo "(test exit code: ${PIPESTATUS[0]})"
