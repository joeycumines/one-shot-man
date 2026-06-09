#!/bin/bash
# mock_claude.sh — simulated Claude Code PTY for autopilot testing
#
# This script simulates a Claude Code-like process that the autopilot can
# wrap and monitor. It produces recognizable output patterns:
#   - Startup banner with version info
#   - Prompt line ("> ") for idle detection
#   - "Working..." with spinner for loading detection
#   - "network error" when "error" is sent as input
#   - "timeout" when "timeout" is sent as input
#   - Exits gracefully on "exit" input or EOF
#
# Usage: bash mock_claude.sh

print_banner() {
    echo "Claude Code (mock)"
    echo "Version: 0.1.0-test"
    echo "Project: /tmp/test-project"
    echo ""
}

print_prompt() {
    printf "> "
}

main() {
    print_banner
    print_prompt

    while IFS= read -r line; do
        case "$line" in
            "error")
                echo ""
                echo "Error: network error occurred"
                echo "Retrying..."
                ;;
            "timeout")
                echo ""
                echo "Error: connection timeout"
                ;;
            "stuck")
                echo ""
                printf "Working "
                for s in ⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧; do
                    printf "\rWorking %s" "$s"
                    sleep 0.05
                done
                echo ""
                echo "Completed."
                ;;
            "exit")
                echo ""
                echo "Exiting Claude Code..."
                break
                ;;
            "")
                echo ""
                echo "Received empty input."
                ;;
            *)
                echo ""
                echo "Echo: $line"
                echo "Type 'exit' to quit, 'error' for network error, 'stuck' for spinner."
                ;;
        esac
        print_prompt
    done

    echo "Done."
}

main
