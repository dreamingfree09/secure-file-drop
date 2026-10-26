"""Codex-only preparation of a new private, one-use context. No live adoption.
Default contexts stay disabled; --enable still requires exact independent review.
The Owner is never asked to run this or provide credentials to it.
"""
import argparse,hashlib,json,os,secrets,shutil,stat,time,sys
from pathlib import Path
from policy import *
from runtime import CODE_PATHS,Runtime,command,PS

def prepare(binding_path,enable=False):
    os.umask(0o077);need(os.getuid()==1000,'preparer-owner')
    binding=private_json(binding_path);source=Path(__file__).resolve().parent
    profile=command([PS,'-NoProfile','-Command','[Environment]::GetFolderPath("UserProfile")']).decode().strip()
    profile=Path(command(['wslpath','-u',profile]).decode().strip())
    tools=profile/'SecureFileDrop-Staged-Acceptance-20261026-593cd1a8'
    need(Path(binding['artifacts']['native_form']).resolve()==(tools/'OwnerStagedInput.exe').resolve() and
         digest(tools/'OwnerStagedInput.exe')==PINS['native_form'] and not tools.is_symlink(),'preparer-Windows-tool-path')
    hashes={p:digest(source/p) for p in CODE_PATHS};bundle=canonical(hashes)
    need(binding['bundle_sha256']==bundle,'preparer-source-bundle')
    name='sfd-controller-'+datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')+'-'+secrets.token_hex(6)
    parent=Path.home()/'.local/share/secure-file-drop/operator-acceptance/controller-sessions'
    parent.mkdir(parents=True,exist_ok=True,mode=0o700);need(parent.stat().st_uid==1000 and stat.S_IMODE(parent.stat().st_mode)==0o700,'session-parent')
    root=parent/name;root.mkdir(mode=0o700);code=root/'controller-code';code.mkdir(mode=0o700)
    for rel in CODE_PATHS:
        src=source/rel;dst=code/rel;dst.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
        need(not dst.exists(),'new-controller-code');dst.write_bytes(src.read_bytes());dst.chmod(0o500)
    probe=root/'storage-probe';need(digest(binding['artifacts']['probe'])==binding['probe_sha256'],'prepared-probe')
    probe.write_bytes(Path(binding['artifacts']['probe']).read_bytes());probe.chmod(0o500)
    binding['artifacts']['probe']=str(probe)
    binding['controller_driver_sha256']=digest(code/'staged_driver.py')
    tools=Path(binding['artifacts']['native_form']).parent
    sessions=tools/'Sessions';sessions.mkdir(mode=0o700,exist_ok=True);need(not sessions.is_symlink(),'Windows-session-parent')
    windows=sessions/name;windows.mkdir(mode=0o700)
    tls=windows/'strict_tls.ps1';tls.write_bytes((code/'strict_tls.ps1').read_bytes())
    need(digest(tls)==hashes['strict_tls.ps1'],'local-Windows-TLS-script')
    cfg=dict(binding,enabled=bool(enable),actual_master=MASTER,operator_reference_master=PROTOCOL_MASTER,
             created_epoch=time.time(),expires_epoch=time.time()+3600,controller_nonce=secrets.token_hex(24),
             windows_session=str(windows),work_ledger=str(Path.home()/'.local/share/secure-file-drop/operator-acceptance/work-ledgers'/WORK))
    out=root/'controller-context.json';new_json(out,cfg);out.chmod(0o400)
    if enable:
        # Validate the exact reviewed remote source/CI/decision before recommending
        # launch; production main.py performs all checks again under its lease.
        command([sys.executable,'-B',str(code/'main.py'),'--context',str(out),'--verify-only'],timeout=90)
    return {'context':str(out),'entrypoint':str(code/'main.py'),'enabled':bool(enable),
            'actual_master':MASTER,'bundle_sha256':bundle,'Owner_input_or_live_mutation':False}
def main():
    p=argparse.ArgumentParser();p.add_argument('--binding',required=True);p.add_argument('--enable',action='store_true');a=p.parse_args()
    try:print(json.dumps(prepare(a.binding,a.enable)));return 0
    except Exception:print('PREPARATION STOPPED — no credential input or live adoption.');return 1
if __name__=='__main__':raise SystemExit(main())
