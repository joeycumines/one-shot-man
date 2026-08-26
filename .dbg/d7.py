import pty,time,signal,subprocess,fcntl,termios,struct,select,threading,os
master,slave=pty.openpty()
fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,100,0,0))
out=open('/Users/joeyc/dev/one-shot-man/.dbg/full.txt','wb')
open('/Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt','w').write('hi\n')
env=dict(os.environ);env['TERM']='xterm-256color';env['EDITOR']=':';env['OSM_CLIPBOARD']='/bin/cat';env['GOTRACEBACK']='all'
proc=subprocess.Popen(['/Users/joeyc/dev/one-shot-man/.dbg/osm_dbg','prompt-flow','-i'],stdin=slave,stdout=slave,stderr=slave,
    preexec_fn=lambda:(os.setsid(),fcntl.ioctl(0,termios.TIOCSCTTY,0)),env=env,close_fds=True)
print("PID",proc.pid)
def rd():
    while True:
        r,_,_=select.select([master],[],[],0.3)
        if master in r:
            try: d=os.read(master,8192)
            except OSError: return
            if not d: return
            out.write(d); out.flush()
threading.Thread(target=rd,daemon=True).start()
def rt():
    time.sleep(2); 
    try: os.write(master,b'add /Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt\n')
    except OSError as e: print("wr1err",e)
    time.sleep(3)
    try: os.write(master,b'list\n')
    except OSError as e: print("wr2err",e)
threading.Thread(target=rd,daemon=True).start()
threading.Thread(target=rt,daemon=True).start()
time.sleep(7)
os.kill(proc.pid,signal.SIGQUIT)
time.sleep=3
time.sleep(3)
out.flush(); out.close()
os._exit(0)
