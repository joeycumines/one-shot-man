#!/bin/bash
# mock_shell.sh — simulated shell for bouncing-logo integration testing
#
# This script simulates an interactive shell that the bouncing-logo demo can
# wrap and monitor. It produces recognizable output patterns:
#   - Startup banner with version info
#   - Prompt line ("> ") for interactive detection
#   - Echoes all input back prefixed with "Echo: "
#   - Exits gracefully on "exit" input or EOF
#
# Usage: bash mock_shell.sh

print_banner() {
    echo "Bouncing Logo Shell (mock)"
    echo "Version: 0.1.0-test"
    echo "Type 'exit' to quit."
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
            "exit")
                echo ""
                echo "Exiting..."
                break
                ;;
            "")
                echo ""
                echo "Received empty input."
                ;;
            *)
                echo ""
                echo "Echo: $line"
                ;;
        esac
        print_prompt
    done

    echo "Done."
}

main
