Output script-example-bt-shooter.gif

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
# Run osm script for BT Shooter game
Type "osm script ../../../scripts/example-04-bt-shooter.js"
Enter
Sleep 500ms
Sleep 300ms
# Press SPACE to start the game
Sleep 500ms
# Move UP and shoot
Up
Up
Up
Up
Up
Up
Up
Up
Sleep 500ms
# Move RIGHT and shoot
Right
Right
Right
Right
Right
Right
Sleep 500ms
# Move DOWN and shoot
Down
Down
Down
Down
Down
Down
Sleep 500ms
# Move LEFT and shoot
Left
Left
Left
Left
Left
Left
Sleep 500ms
# Quit the game
Type "q"
Sleep 300ms
Type "exit"
Enter
