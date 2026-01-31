#!/bin/bash
# Calculate elapsed time since session start
set -e

START_TIME="2026-01-30T19:25:43Z"
CURRENT_TIME=$(date "+%Y-%m-%dT%H:%M:%SZ")

# Convert to seconds since epoch
START_SEC=$(date -j -f "%Y-%m-%dT%H:%M:%SZ" "$START_TIME" "+%s")
CURRENT_SEC=$(date -j -f "%Y-%m-%dT%H:%M:%SZ" "$CURRENT_TIME" "+%s")

# Calculate elapsed seconds
ELAPSED_SEC=$((CURRENT_SEC - START_SEC))

# Convert to hours, minutes, seconds
HOURS=$((ELAPSED_SEC / 3600))
MINUTES=$(((ELAPSED_SEC % 3600) / 60))
SECONDS=$((ELAPSED_SEC % 60))

echo "Start: $START_TIME"
echo "Current: $CURRENT_TIME"
echo "Elapsed: ${HOURS}h ${MINUTES}m ${SECONDS}s"

# Calculate remaining hours for 4-hour mandate (4 hours = 14400 seconds)
TOTAL_MANDATE_SEC=$((4 * 3600))
REMAINING_SEC=$((TOTAL_MANDATE_SEC - ELAPSED_SEC))

if [ $REMAINING_SEC -le 0 ]; then
    echo "Mandate exceeded! Over by: $(((-REMAINING_SEC) / 3600))h $((((-REMAINING_SEC) % 3600) / 60))m"
else
    REMAINING_HOURS=$((REMAINING_SEC / 3600))
    echo "Remaining to meet 4-hour mandate: ${REMAINING_HOURS}h"
fi

echo "${HOURS}h ${MINUTES}m ${SECONDS}s"
