"""Separate operator workflow; live execution needs a pinned independent decision.
No service update/restoration is performed here. The governed controller remains
responsible for fresh preservation gates, exclusive recovery and backend adoption.
Only a normal /register + actually delivered /verify creates/verifies the account.
"""
from pathlib import Path
import datetime,email,email.utils,gzip,hashlib,imaplib,json,os,re,secrets,select,ssl,stat,subprocess,sys,time,urllib.parse,uuid

WORK='SFD-OWNER-TEST-ACCOUNT-20261026'
MASTER='ae45cb816f9ec05e334e4620a4b428cebe631901'
IMAGE='sha256:084afb9ff9ecd6244f24e795e86f35cd8d328130915bc44aa0d96ebfe0e356f2'
F196='f196bab87a426d6a5d287f16c199259edb7993e15c4e35d2886d4a196a6fa0ca'
NAME=re.compile(r'sfd_acceptance_20261026(?:_[a-f0-9]{8})?\Z')
def need(value):
    if not value:raise RuntimeError('guard failed')
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def new_json(p,value):
    fd=os.open(p,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    with os.fdopen(fd,'w') as f:json.dump(value,f);f.write('\n');f.flush();os.fsync(f.fileno())
def private_json(p):
    p=Path(p);s=p.lstat()
    need(stat.S_ISREG(s.st_mode) and s.st_uid==os.getuid() and not(stat.S_IMODE(s.st_mode)&0o077))
    return json.loads(p.read_text())
def run(args,**kw):
    p=subprocess.run(args,capture_output=True,timeout=kw.pop('timeout',25),**kw)
    need(p.returncode==0);return p.stdout
def services():
    ids=run(['docker','ps','-aq','--filter','label=com.docker.compose.project=securefiledrop']).decode().split()
    need(len(ids)==6);cs=json.loads(run(['docker','inspect',*ids]));need(all(c['State']['Running'] for c in cs));return cs
def env(c):return dict(v.split('=',1) for v in c['Config']['Env'] if '=' in v)
def password():return 'Sfd9!'+secrets.token_urlsafe(32)
def verify_path(raw):
    u=urllib.parse.urlsplit(raw)
    need(u.scheme=='https' and u.hostname=='localhost' and u.port==8443 and not u.username and not u.password and not u.fragment and u.path=='/verify')
    q=urllib.parse.parse_qs(u.query,strict_parsing=True)
    need(set(q)=={'token'} and len(q['token'])==1 and re.fullmatch('[a-f0-9]{64}',q['token'][0]))
    return '/verify?'+urllib.parse.urlencode({'token':q['token'][0]})
def mail_path(raw,recipient):
    m=email.message_from_bytes(raw)
    need([a.lower() for _,a in email.utils.getaddresses(m.get_all('To',[]))]==[recipient.lower()])
    texts=[]
    for p in m.walk():
        if p.get_content_type() in ['text/plain','text/html']:
            b=p.get_payload(decode=True) or b''
            need(len(b)<131072);texts.append(b.decode(p.get_content_charset() or 'utf-8'))
    links=set()
    for t in texts:
        for link in re.findall(r'https://[^\s<>"\x27]+',t):
            try:links.add(verify_path(link.replace('&amp;','&')))
            except (RuntimeError,ValueError):pass
    need(len(links)==1);return links.pop()

class Transport:
    def __init__(self,path):
        self.p=subprocess.Popen([str(path)],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL)
    def call(self,x):
        self.p.stdin.write(json.dumps(x).encode()+b'\n');self.p.stdin.flush()
        # Windows interop streams cannot use select reliably; a watchdog bounds
        # the whole child lifetime independently of the per-request HTTP budget.
        import threading
        guard=threading.Timer(45,self.p.kill);guard.start()
        try:
            raw=self.p.stdout.readline(65537);need(len(raw)<=65536);r=json.loads(raw);need(r.get('ok'));return r
        finally:guard.cancel()
    def close(self):
        try:self.p.stdin.close();self.p.wait(timeout=3)
        except Exception:self.p.kill();self.p.wait(timeout=5)
        self.p.stdout.close()

def probe(cfg,cs,values):
    b=next(c for c in cs if c['Name']=='/sfd_backend');pg=next(c for c in cs if c['Name']=='/sfd_postgres')
    shared=set(b['NetworkSettings']['Networks'])&set(pg['NetworkSettings']['Networks']);need(len(shared)==1);network=shared.pop()
    u=urllib.parse.urlsplit(env(b)['DATABASE_URL'])
    aliases=set(pg['NetworkSettings']['Networks'][network].get('Aliases') or [])|{pg['Name'].lstrip('/'),pg['NetworkSettings']['Networks'][network]['IPAddress']}
    need(u.hostname in aliases and u.port in [None,5432])
    need('backend' in b['NetworkSettings']['Networks'][network]['Aliases'])
    owner='sfd-staged-account-'+secrets.token_hex(6);label='sfd.staged-account'
    args=['docker','run','--rm','-i','--pull=never','--name',owner,'--label',label+'='+owner,'--network',network,'--user',str(os.getuid())+':'+str(os.getgid()),'--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--pids-limit','64','--memory','128m','--mount','type=bind,source='+cfg['operator_binary']+',target=/probe/check,readonly','--entrypoint','/probe/check',IMAGE]
    try:return json.loads(run(args,input=json.dumps(values).encode(),timeout=25))
    finally:
        found=subprocess.run(['docker','inspect',owner],capture_output=True)
        if found.returncode==0:
            c=json.loads(found.stdout)[0];need(c['Config']['Labels'].get(label)==owner and c['Image']==IMAGE)
            run(['docker','rm','-f',c['Id']])
def line_input():
    need(select.select([sys.stdin],[],[],600)[0])
    raw=sys.stdin.buffer.readline(16385);need(len(raw)<=16384);return json.loads(raw)
def decision(cfg):
    need(cfg['master']==MASTER and cfg['candidate']==IMAGE and cfg['execution_authorised'] is True)
    need(re.fullmatch(r'https://github.com/dreamingfree09/secure-file-drop/pull/35#issuecomment-[0-9]+',cfg['independent_review']))
    need(cfg['independent_review']!='https://github.com/dreamingfree09/secure-file-drop/pull/35#issuecomment-6413982287')
    for p,h in cfg['artifacts'].items():need(sha(p)==h)
    need(sha(cfg['release'])==F196)
    need(sha(__file__)==cfg['driver_sha256'])
    # The controller supplies an exact-hash, fresh non-secret gate receipt.
    gates=private_json(cfg['gates_receipt']);need(sha(cfg['gates_receipt'])==cfg['gates_sha256'])
    need(gates['all_mandatory_gates_passed'] is True and gates['reviewed_three_original_scope'] is True)
    age=time.time()-datetime.datetime.fromisoformat(gates['UTC']).timestamp();need(0<=age<180)
    need(gates['scheduler_margin_seconds']>=5400 and gates['Owner_credentials_requested'] is False)

def parent_alive(pid,expected_hash):
    need(isinstance(pid,int) and pid>0 and re.fullmatch('[a-f0-9]{64}',expected_hash))
    command="$ErrorActionPreference='Stop'; $p=Get-Process -Id "+str(pid)+"; if($p.MainWindowHandle -eq 0){exit 3}; if((Get-FileHash -LiteralPath $p.Path -Algorithm SHA256).Hash.ToLowerInvariant() -cne '"+expected_hash+"'){exit 4}"
    run(['/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe','-NoProfile','-Command',command])

def main(cfgpath):
    os.umask(0o077);need(os.getuid()!=0 and not os.environ.get('DOCKER_HOST'))
    cfg=private_json(cfgpath);root=Path(cfg['state_directory'])
    need(root.stat().st_uid==os.getuid() and stat.S_IMODE(root.stat().st_mode)==0o700)
    decision(cfg)
    need(run(['docker','context','inspect','--format','{{.Endpoints.docker.Host}}']).decode().strip()=='unix:///var/run/docker.sock')
    need(not any((root/p).exists() for p in ['owner-ready.json','backend-adopted.json','result.json']))
    cs=services();b=next(c for c in cs if c['Name']=='/sfd_backend')
    need({c['Name']:c['Id'] for c in cs}==cfg['initial_service_ids'] and b['Id']==cfg['old_backend_id'])
    original_env=env(b)
    supplied=line_input();need(supplied['idle'] is True and int(supplied['Windows_PID'])>0)
    admin=supplied['admin'];windows_pid=int(supplied['Windows_PID']);supplied.clear();parent_alive(windows_pid,cfg['native_form_sha256'])
    answer=probe(cfg,cs,{'Mode':'check-admin-password','AdminHash':original_env['SFD_ADMIN_PASS'],'AdminPassword':admin});need(answer['ok'])
    # Availability and preservation snapshot use only a read-only transaction.
    snap=probe(cfg,cs,{'Mode':'snapshot','DSN':original_env['DATABASE_URL']});need(snap['ok'])
    nonce=secrets.token_hex(24);deadline=time.time()+1200
    new_json(root/'owner-ready.json',{'work_id':WORK,'pid':os.getpid(),'Windows_PID':windows_pid,'nonce':nonce,'master':MASTER,'candidate':IMAGE,'expires_utc':datetime.datetime.fromtimestamp(deadline,datetime.timezone.utc).isoformat(),'administrator_password_match':True,'Owner_confirmed_IDLE':True,'registered_password_requested':False,'existing_users_sha256':snap['existing_users_sha256']})
    print('ADMIN_VERIFIED_WAITING_FOR_ADOPTION',flush=True)
    while time.time()<deadline and not(root/'backend-adopted.json').exists():
        parent_alive(windows_pid,cfg['native_form_sha256']);time.sleep(2)
    need(time.time()<deadline);parent_alive(windows_pid,cfg['native_form_sha256'])
    adopted=private_json(root/'backend-adopted.json');need(adopted['nonce']==nonce and adopted['master']==MASTER and adopted['candidate']==IMAGE and adopted['schema']==12)
    cs=services();b=next(c for c in cs if c['Name']=='/sfd_backend');e=env(b)
    need(b['Id']==adopted['backend_id'] and b['Image']==IMAGE and b['Id']!=cfg['old_backend_id'])
    need(all(c['Id']==cfg['initial_service_ids'][c['Name']] for c in cs if c['Name']!='/sfd_backend'))
    need(e==dict(original_env,SFD_BACKUP_RETENTION_DAYS='30') and b['Config'].get('StopTimeout')==25)
    t=Transport(Path(cfg['strict_transport']))
    pwd=None;account=None;stage='genuine post-adoption administrator authentication'
    try:
        need(t.call({'op':'policy'})['revocation'])
        a=t.call({'op':'admin','username':e['SFD_ADMIN_USER'],'password':admin});session=a['session'];a.clear()
        stage='controlled mailbox'
        # Exact mailbox identity was verified separately; never use an arbitrary recipient.
        need(e['SFD_SMTP_USER'].casefold()==cfg['controlled_mailbox'].casefold() and e['SFD_SMTP_FROM'].casefold()==cfg['controlled_mailbox'].casefold())
        local,domain=cfg['controlled_mailbox'].rsplit('@',1)
        recipient=local.split('+',1)[0]+'+sfda20261026'+secrets.token_hex(4)+'@'+domain
        username='sfd_acceptance_20261026'
        # Do not choose a name by making mutating registration attempts. This
        # exact privately reviewed selection is checked in the fresh gate receipt.
        gates=private_json(cfg['gates_receipt'])
        username=gates['available_acceptance_username'];need(NAME.fullmatch(username))
        with imaplib.IMAP4_SSL('imap.gmail.com',993,ssl_context=ssl.create_default_context(),timeout=15) as m:
            m.login(e['SFD_SMTP_USER'],e['SFD_SMTP_PASS'])
            typ,_=m.select('INBOX',readonly=True);need(typ=='OK')
            start=int(m.response('UIDNEXT')[1][0])
            validity=m.response('UIDVALIDITY')[1][0]
            pwd=password();stage='normal registration';parent_alive(windows_pid,cfg['native_form_sha256'])
            new_json(root/'account-attempt.json',{'username':username,'email':recipient,'password_persisted':False,'creation_confirmed':False,'verification_method':'normal SMTP and verify endpoint'})
            r=t.call({'op':'register','body':{'username':username,'email':recipient,'password':pwd}})
            need(r['status']==201);account=r['body'];need(str(uuid.UUID(account['id']))==account['id'] and account['username']==username and account['email']==recipient)
            stage='delivered verification message';end=time.monotonic()+120;path=None
            while time.monotonic()<end:
                parent_alive(windows_pid,cfg['native_form_sha256'])
                typ,rows=m.uid('search',None,'UID',str(start)+':*','TO','"'+recipient+'"');need(typ=='OK')
                candidates=[int(v) for v in rows[0].split() if int(v)>=start];need(len(candidates)<=2)
                for uid in candidates:
                    typ,data=m.uid('fetch',str(uid),'(BODY.PEEK[])');need(typ=='OK')
                    raw=next(v[1] for v in data if isinstance(v,tuple));need(len(raw)<=131072)
                    path=mail_path(raw,recipient)
                if path:break
                time.sleep(2)
            need(path is not None and validity is not None)
            stage='normal email verification';need(t.call({'op':'verify','path':path})['status']==200);path=None
        stage='reviewed cap of new ordinary account';parent_alive(windows_pid,cfg['native_form_sha256'])
        capped=probe(cfg,cs,{'Mode':'cap-new-account','DSN':e['DATABASE_URL'],'AdminName':e['SFD_ADMIN_USER'],'Session':session,'BackendOrigin':'http://backend:8080','ID':account['id'],'Username':username,'Email':recipient,'ExistingUsersSHA256':snap['existing_users_sha256']})
        need(capped['ok'] and capped['quota_bytes']==1048576);session=None
        need(t.call({'op':'logout'})['ok'])
        stage='genuine F196 administrator and ordinary acceptance'
        private={'Origin':'https://localhost:8443','AdminName':e['SFD_ADMIN_USER'],'AdminPassword':admin,'RegisteredLogin':recipient,'RegisteredName':username,'RegisteredPassword':pwd,'PayloadPath':cfg['Windows_payload']}
        executed=run([cfg['live_acceptance']],input=json.dumps(private).encode(),timeout=120);private.clear();admin=None
        result=json.loads(executed);need(result['success'])
        result.update(work_id=WORK,username=username,account_id=account['id'],email_verification='actual controlled SMTP delivery and normal verify endpoint',quota_bytes=1048576,password_persisted=False,Owner_visual_acceptance=False)
        new_json(root/'result.json',result)
        # Only the WinExe parent's private redirected pipe receives this secret.
        # It is never printed by a shell, recorded as evidence or sent to Codex.
        parent_alive(windows_pid,cfg['native_form_sha256'])
        print(json.dumps({'event':'ACCOUNT_ACTIVE','username':username,'password':pwd}),flush=True)
        pwd=None
        return 0
    except Exception:
        new_json(root/'result.json',{'success':False,'stage':stage,'account_id':account.get('id') if account else None,'username':account.get('username') if account else None,'password_persisted':False,'automatic_retry_reset_or_deletion':False})
        return 1
    finally:
        admin=pwd=None
        try:t.call({'op':'logout'})
        except Exception:pass
        t.close()
if __name__=='__main__':
    try:raise SystemExit(main(sys.argv[1]))
    except Exception:print('SECURE_WORKFLOW_STOPPED',flush=True);raise SystemExit(1)
