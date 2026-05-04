#!/usr/bin/env bash
# snapshot.sh — Capture the current working-tree diff as a patch file and create a backup branch.
# Usage: source <(./scripts/snapshot.sh)
# Outputs key-value pairs for ledger initialization.

set -euo pipefail

TIMESTAMP=$(date +%Y%m%d-%H%M%S)
SCRATCH_DIR="./scratch"
PATCH_NAME="commit-loom-snapshot-${TIMESTAMP}.patch"
PATCH_PATH="${SCRATCH_DIR}/${PATCH_NAME}"
BACKUP_BRANCH="commit-loom-backup-${TIMESTAMP}"
CURRENT_BRANCH=$(git branch --show-current 2>/dev/null || echo "detached")
CURRENT_HEAD=$(git rev-parse HEAD)

mkdir -p "${SCRATCH_DIR}"

# Capture the full working-tree diff vs HEAD
git diff HEAD > "${PATCH_PATH}"

# Also capture diffstat for summary
git diff HEAD --stat > "${SCRATCH_DIR}/commit-loom-stat-${TIMESTAMP}.txt"

# Create backup branch pointing at current HEAD (does not switch to it)
git branch "${BACKUP_BRANCH}" HEAD 2>/dev/null || true

# Count hunks and files
HUNK_COUNT=$(grep -c '^@@' "${PATCH_PATH}" 2>/dev/null || echo "0")
FILE_COUNT=$(git diff HEAD --name-only | wc -l | tr -d ' ')
INSERTIONS=$(git diff HEAD --shortstat | grep -o '[0-9]* insertion' | grep -o '[0-9]*' || echo "0")
DELETIONS=$(git diff HEAD --shortstat | grep -o '[0-9]* deletion' | grep -o '[0-9]*' || echo "0")

cat <<EOF
SNAPSHOT_TIMESTAMP=${TIMESTAMP}
SNAPSHOT_PATCH=${PATCH_PATH}
SNAPSHOT_PATCH_NAME=${PATCH_NAME}
BACKUP_BRANCH=${BACKUP_BRANCH}
CURRENT_BRANCH=${CURRENT_BRANCH}
CURRENT_HEAD=${CURRENT_HEAD}
HUNK_COUNT=${HUNK_COUNT}
FILE_COUNT=${FILE_COUNT}
INSERTIONS=${INSERTIONS}
DELETIONS=${DELETIONS}
EOF
