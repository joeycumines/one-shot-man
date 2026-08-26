import pty,time,signal,subprocess,fcntl,termios,struct,threading,os
master,slave=pty.openpty()
fcntl.ioctl(slave,termios.TIOCSWINSZ,struct.pack('HHHH',24,100,0,0))
so=open('/Users/joeyc/dev/one-shot-man/.dbg/so.txt','wb'); se=open('/Users/joeyc/dev/one-shot-man/.dbg/se.txt','wb')
open('/Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt','w').write('hi\n')
env=dict(os.environ);env['TERM']='xterm-256color';env['EDITOR']=':';env['OSM_CLIPBOARD']='/bin/cat';env['GOTRACEBACK']='all'
def pre():
    os.setsid(); os.dup2(slave,0); fcntl.ioctl(0,termios.TIOCSCTTY,0)
proc=subprocess.Popen(['/Users/joeyc/dev/one-shot-man/.dbg/osm_dbg','prompt-flow','-i'],preexec_fn=pre,stdin=slave,stdout=so,stderr=se,env=env,close_fds=True)
print("PID",proc.pid,flush=True)
proc.stdout=None; proc.stderr=None
def rt():
    time.sleep(2)
    try: os.write(master,b'add /Users/joeyc/dev/one-shot-man/.dbg/ws/gone.txt\n')
    except OSError as e: pass
    time.sleep(3)
    try: os.write(master,b'list\n')
    except OSError as e: pass
os.write(master,b'')
threading.Thread(target=rt,daemon=True).start()
time.sleep(7)
se.flush(); so.flush()
print("alive?",proc.poll() is None,flush=True)
os.kill(proc.pid,signal.SIGQUIT)
time.sleep=time.sleep
time.sleep(4)
se.flush(); so.flush()
os._exit(0)
