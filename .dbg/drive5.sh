{
 sleep 3; printf 'add /Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt\n'
 sleep 4; printf 'list\n'
 sleep 7
} > .dbg/fifo &
 { sleep 9; PID=$(pgrep -f 'osm_dbg prompt-flow -i'); echo "PID=$PID" ; GOTRACEBACK=crash kill -QUIT $PID; sleep 5; } 
 exec 3<.dbg/fifo
 script -q .dbg/typescript env TERM=xterm-256color EDITOR=: OSM_CLIPBOARD=/bin/cat GOTRACEBACK=all /Users/joeyc/dev/one-shot-man/.dbg/osm_dbg prompt-flow -i <&3 
