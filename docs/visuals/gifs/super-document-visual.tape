Output super-document-visual.gif

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
# Run osm super-document
Type "osm super-document"
Enter
Sleep 500ms
# Add first document
Type "a"
Tab
Type "# Project Requirements"
Enter
Enter
Type "- User authentication"
Enter
Type "- Data persistence"
Enter
Type "- API endpoints"
Tab
Enter
Sleep 400ms
# Add second document
Type "a"
Tab
Type "# Technical Specs"
Enter
Enter
Type "Backend: Go with Goja JS runtime"
Enter
Type "Frontend: Terminal UI (Bubble Tea)"
Tab
Enter
Sleep 400ms
# Navigate between documents
Down
Sleep 300ms
Up
Sleep 300ms
# Copy the combined prompt
Type "c"
Sleep 600ms
# Quit
Type "q"
Sleep 300ms
Type "exit"
Enter
