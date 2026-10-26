"""Private WinExe child only. Hold generated credentials in RAM until controller verdict.
No password is written to files, arguments, environment, diagnostics or terminal.
"""
import json,os,secrets,subprocess,sys,time
from pathlib import Path
from policy import GateFailure,need,private_json,new_json,digest,PINS,MASTER
class HandoffBuffer:
    def __init__(self,line,username):
        v=json.loads(line)
        need(set(v)=={'event','username','password'} and v['event']=='ACCOUNT_ACTIVE' and v['username']==username,'broker-event')
        need(isinstance(v['password'],str) and 40<=len(v['password'].encode())<=72,'broker-password-shape')
        self.data=bytearray(line);v.clear()
    def release(self,verdict,nonce,master):
        need(verdict.get('success') is True and verdict.get('nonce')==nonce and verdict.get('actual_master')==master,'handoff-verdict')
        data=bytes(self.data);self.clear();return data
    def clear(self):
        for i in range(len(self.data)):self.data[i]=0
        self.data.clear()
def main(path):
    cfg=private_json(path);root=Path(cfg['state_directory'])
    need(cfg['execution_authorised'] is True and cfg['actual_master']==MASTER,'broker-enabled')
    need(digest(cfg['controller_context'])==cfg['controller_context_sha256'],'broker-controller-context')
    start=private_json(root/'controller-started.json')
    need(start['context_nonce']==cfg['controller_nonce'] and start['context_sha256']==cfg['controller_context_sha256'],'broker-controller-lease')
    def controller_alive():
        p=Path('/proc')/str(start['pid'])
        need(p.stat().st_uid==os.getuid() and (p/'cmdline').read_bytes().split(b'\x00').count(str(Path(__file__).with_name('main.py')).encode())==1,'controller-process')
    controller_alive()
    from runtime import Runtime
    Runtime(cfg['controller_context']).authority(existing_session=True)
    source=Path(cfg['operator_driver']);need(digest(source)==private_json(cfg['controller_context'])['controller_driver_sha256'],'broker-controller-driver')
    from importlib.util import spec_from_file_location,module_from_spec
    spec=spec_from_file_location('frozen_operator',source);driver=module_from_spec(spec);spec.loader.exec_module(driver)
    owner=sys.stdin.buffer.readline(16385);need(len(owner)<=16384,'broker-input')
    value=json.loads(owner);need(value.get('idle') is True,'broker-idle');windows_pid=int(value['Windows_PID'])
    driver.parent_alive(int(value['Windows_PID']),PINS['native_form'])
    new_json(root/'owner-present.json',{'pid':os.getpid(),'Windows_PID':windows_pid,'idle':True,
             'epoch':time.time(),'controller_nonce':cfg['controller_nonce']})
    gate_end=time.monotonic()+180
    while time.monotonic()<gate_end:
        controller_alive();driver.parent_alive(windows_pid,PINS['native_form'])
        if (root/'abort.json').exists():raise GateFailure('controller-abort')
        if (root/'operator-ready.json').exists():break
        time.sleep(.5)
    else:raise GateFailure('fresh-operator-gates-timeout')
    control=private_json(root/'operator-ready.json')
    need(control['controller_nonce']==cfg['controller_nonce'] and digest(cfg['operator_context'])==control['context_sha256'],'fresh-operator-context')
    operator_cfg=private_json(cfg['operator_context']);driver.decision(operator_cfg)
    child=subprocess.Popen([sys.executable,'-B',str(source),cfg['operator_context']],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL)
    held=None
    try:
        child.stdin.write(owner);child.stdin.flush();owner=None;value.clear()
        new_json(root/'broker-started.json',{'pid':os.getpid(),'driver_pid':child.pid,'actual_master':MASTER})
        deadline=time.monotonic()+1800
        import select
        while time.monotonic()<deadline:
            if (root/'abort.json').exists():raise GateFailure('controller-abort')
            controller_alive();driver.parent_alive(windows_pid,PINS['native_form'])
            if not select.select([child.stdout],[],[],2)[0]:continue
            line=child.stdout.readline(65537);need(len(line)<=65536,'broker-output-bound')
            if not line:break
            if line.startswith(b'{'):
                name=private_json(cfg['gates_receipt'])['available_acceptance_username']
                need(held is None,'broker-single-event');held=HandoffBuffer(line.rstrip(b'\n'),name);line=None;break
            if line.strip()==b'ADMIN_VERIFIED_WAITING_FOR_ADOPTION':
                sys.stdout.write('ADMIN_VERIFIED_WAITING_FOR_ADOPTION\n');sys.stdout.flush()
            elif line.strip()==b'SECURE_WORKFLOW_STOPPED':raise GateFailure('operator-stopped')
        need(held is not None,'broker-no-success')
        child.wait(timeout=10);need(child.returncode==0,'operator-exit')
        ready=private_json(root/'owner-ready.json');deadline=time.monotonic()+180
        while time.monotonic()<deadline:
            controller_alive();driver.parent_alive(ready['Windows_PID'],PINS['native_form'])
            if (root/'abort.json').exists():raise GateFailure('controller-abort')
            if (root/'handoff-approved.json').exists():
                result=private_json(root/'result.json');need(result.get('success') is True and result.get('count')==27,'broker-result')
                verdict=private_json(root/'handoff-approved.json')
                need(time.time()-verdict['epoch']<60,'handoff-fresh')
                data=held.release(verdict,ready['nonce'],MASTER)
                sys.stdout.buffer.write(data+b'\n');sys.stdout.buffer.flush();data=None
                new_json(root/'handoff-delivered.json',{'success':True,'username':result['username'],'password_persisted':False})
                return 0
            time.sleep(.5)
        raise GateFailure('handoff-expired')
    finally:
        if held:held.clear()
        if child.poll() is None:child.terminate()
        try:child.wait(timeout=5)
        except subprocess.TimeoutExpired:child.kill();child.wait(timeout=5)
        child.stdin.close();child.stdout.close()
if __name__=='__main__':
    try:raise SystemExit(main(sys.argv[1]))
    except Exception:
        try:
            cfg=private_json(sys.argv[1]);p=Path(cfg['state_directory'])/'broker-stopped.json'
            if not p.exists():new_json(p,{'success':False,'password_persisted':False})
        except Exception:pass
        print('SECURE_WORKFLOW_STOPPED',flush=True);raise SystemExit(1)
