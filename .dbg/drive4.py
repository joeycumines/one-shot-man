import pty, time, signal, subprocess, fcntl, termios, struct, select, threading, os
master, slave = pty.openpty()
fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH',24,100,0,0))
env = dict(os.environ); env['TERM']='xterm-256color'; env['EDITOR']=':'; env['GOTRACEBACK']='all'
open('/Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt','w').write('hi\n')
env['OSM_CLIPBOARD']='cat /dev/null'
def pre():
    os.setsid()
    import io
    os.dup2(slave,0); os.dup2(slave,1); os.dup2(slave,2)
    fcntl.ioctl(0, termios.TIOCSCTTY, 0)
proc = subprocess.Popen(['/Users/joeyc/dev/one-shot-man/.dbg/osm_dbg','prompt-flow','-i'],
    preexec_fn=pre, close_fds=True, env=env)
def reader():
    f=open('/Users/joeyc/dev/one-shot-man/.dbg/cap4.txt','wb')
    while True:
        r,_,_=select.select([master],[],[],0.2)
        if master in r:
            try:
                d=os.read(master,4096)
            except OSError: break
            if not d: break
            f.write(d); f.flush()
threading.Thread(target=reader,daemon=True).start()
def rt():
    time.sleep(2)
    os.write(master,b'add /Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt\r')
    time.sleep(3)
    os.write(master,b'list\r')
threading.Thread(target=rt,daemon=True).start()
time.sleep(6)
os.kill(proc.pid, signal.SIGQUIT)
time.sleep(3)
os._exit(0)
