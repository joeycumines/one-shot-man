( sleep 3; printf 'add /Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt\r'; sleep 4; printf 'list\r'; sleep 6 ) | script -q /dev/null env TERM=xterm-256color EDITOR=: OSM_CLIPBOARD=cat /Users/joeyc/dev/one-shot-man/.dbg/osm_dbg prompt-flow -i >/Users/joeyc/dev/one-shot-man/.dbg/cap.txt 2>&1 &
DRIVE_PID=$!
sleep 8
PID=$(pgrep -f 'osm_dbg prompt-flow' | head -1)
echo "osm pid=$PID"
kill -QUIT "$PID" 2>/dev/null
wait $DRIVE_PID 2>/dev/null
echo "=== captured ==="
grep -a 'goroutine' /Users/joeyc/dev/one-shot-man/.dbg/cap.txt | head -5
