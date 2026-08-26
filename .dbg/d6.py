import pty,time,signal,subprocess,fcntl,termios,struct,select,threading,os
master,slave=pty.openpty()
fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,100,0,0))
errf=open('/Users/joeyc/dev/one-shot-man/.dbg/stk.txt','wb')
open('/Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt','w').write('hi\n')
env=dict(os.environ);env['TERM']='xterm-256color';env['EDITOR']=':';env['OSM_CLIPBOARD']='/bin/cat'
def pre():
    os.setsid(); os.dup2(slave,0); os.dup2(slave,1); fcntl.ioctl(0,termios.TIOCSCTTY,0)
proc=subprocess.Popen(['/Users/joeyc/dev/one-shot-man/.dbg/osm_dbg','prompt-flow','-i'],preexec_fn=pre,stdin=slave,stdout=slave,stderr=errf,env=env,close_fds=True)
def rd():
    while True:
        r,_,_=select.select([master],[],[],0.3)
        if master in r:
            try: d=os.read(master,4096)
            except OSError: return
            if not d: return
threading.Thread(target=rd,daemon=True).start()
def rt():
    time.sleep(2); os.write(master,b'add /Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt\n'); time.sleep(3); os.write(master,b'list\n')
threading.Thread(target=rt,daemon=True).start()
time.sleep(6)
os.kill(proc.pid,signal.SIGQUIT)
time.sleep(4)
errf.flush()
os._exit(0)
