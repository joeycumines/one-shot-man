Output prompt-flow.gif

# Terminal Settings
Set Width 1000
Set Height 600
Set FontSize 16
Set Shell "bash"
Set TypingSpeed 30ms
Set PlaybackSpeed 0.7
Set WindowBar Colorful
Set Padding 20
Set Margin 10
Set MarginFill "#1a1b26"
Set BorderRadius 8
Set CursorBlink true

# Theme
Set Theme { "name": "OSM Dark", "background": "#1a1b26", "foreground": "#c0caf5", "black": "#15161e", "red": "#f7768e", "green": "#9ece6a", "yellow": "#e0af68", "blue": "#7aa2f7", "magenta": "#bb9af7", "cyan": "#7dcfff", "white": "#a9b1d6", "brightBlack": "#15161e", "brightRed": "#f7768e", "brightGreen": "#9ece6a", "brightYellow": "#e0af68", "brightBlue": "#7aa2f7", "brightMagenta": "#bb9af7", "brightCyan": "#7dcfff", "brightWhite": "#a9b1d6", "cursor": "#c0caf5", "selection": "#44475a" }

# Recorded Actions
Sleep 200ms
# Run osm prompt-flow
Type "osm prompt-flow"
Enter
Sleep 500ms
# Show available commands
Type "help"
Enter
Sleep 800ms
# Set the development goal
Type "goal Implement comprehensive test coverage"
Enter
Sleep 400ms
# Generate the meta-prompt
Type "generate"
Enter
Sleep 400ms
# Show the meta-prompt for LLM
Type "show meta"
Enter
Sleep 600ms
# Set task prompt (from LLM response)
Type "use Write unit tests for authentication handlers"
Enter
Sleep 400ms
# Show the final generated prompt
Type "show"
Enter
Sleep 600ms
# Copy to clipboard
Type "copy"
Enter
Sleep 400ms
Type "exit"
Enter
Sleep 300ms
Type "exit"
Enter
