Output script-example-pick-and-place.gif

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
# Run osm script for Pick-and-Place simulation
Type "osm script ../../../scripts/example-05-pick-and-place.js"
Enter
Sleep 500ms
# Watching automatic PA-BT simulation...
Sleep 50s
Sleep 1s
# Quit the simulation
Type "q"
Sleep 300ms
Type "exit"
Enter
