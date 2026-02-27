// Package statusbar renders a persistent status bar on the terminal's last
// row using ANSI escape sequences and scroll region management.
//
// It manages cursor save/restore, scroll region setup (DECSTBM), and
// reverse-video rendering for the status line content, ensuring child
// process output is confined to the scrollable area above the bar.
package statusbar
