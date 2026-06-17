#!/bin/bash
# mock_shell.sh — simulated shell for bouncing-logo integration testing
#
# Produces recognizable output patterns and echoes all input back prefixed
# with "Echo: ". Intentionally minimal: no prompt, no echo of typed input, so
# the test harness can reliably find the echoed line.

main() {
    # Avoid inheriting a raw/cbreak termios state from a BubbleTea parent PTY.
    stty sane 2>/dev/null || true

    echo "Bouncing Logo Shell (mock)"
    echo "Type 'exit' to quit."

    while IFS= read -r line; do
        case "$line" in
            "exit")
                echo "Exiting..."
                break
                ;;
            "")
                echo "Received empty input."
                ;;
            *)
                printf 'Echo: %s\n' "$line"
                ;;
        esac
    done

    echo "Done."
}

main
