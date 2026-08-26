import pty, time, signal, subprocess, fcntl, termios, struct, select, threading, os
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH',24,100,0,0))
env = dict(os.environ); env['TERM']='xterm-256color'; env['EDITOR']=':'
open('/Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt','w').write('hi\n')
env['OSM_CLIPBOARD']='cat > /dev/null'
proc = subprocess.Popen(['/Users/joeyc/dev/one-shot-man/.dbg/osm_dbg','prompt-flow','-i'], stdin=master, stdout=master, stderr=master, preexec_fn=os.setsid, env=env)
os.close(slave)
def reader():
    while True:
        r,_,_=select.select([master],[],[],0.2)
        if master in r:
            try: d=os.read(master,4096)
            except OSError: return
            if not d: return
threading.Thread(target=reader,daemon=True).start()
def rt():
    time.sleep(1.5); os.write(master,b'add /Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt\n'); time.sleep(3); os.write(master,b'list\n')
threading.Thread(target=rt,daemon=True).start()
time.sleep(3.5)
os.kill(proc.pid, signal.SIGQUIT)
time.sleep(3)
os._exit(0)
