"""Reviewed-source runtime adapter. No arbitrary command hooks or shell execution.
All production mutations are confined to exclusive archive publication and the
accepted backend-only update. Account creation is delegated to frozen PR37 code.
"""
import collections,datetime,fcntl,gzip,hashlib,imaplib,importlib.util,ipaddress,json,os,re,secrets,signal,smtplib,ssl,stat,subprocess,sys,time,uuid
from pathlib import Path
from policy import *

GH='/mnt/c/Program Files/GitHub CLI/gh.exe'
PS='/mnt/c/Windows/System32/WindowsPowerShell/v1.0/powershell.exe'
REPO='repos/dreamingfree09/secure-file-drop/'
CODE_PATHS=['policy.py','runtime.py','broker.py','main.py','prepare_context.py','strict_tls.ps1',
            'staged_driver.py','probe/main.go','probe/guard.go','probe/go.mod','probe/go.sum']
MASTER_CI={'CI':40403320828,'Operator acceptance proposal checks':40403320822,'Windows companion':40403397291}
def command(args,**kw):
    p=subprocess.run(args,capture_output=True,timeout=kw.pop('timeout',30),**kw)
    need(p.returncode==0,'bounded-command');return p.stdout
def environment(c):return dict(v.split('=',1) for v in c['Config']['Env'] if '=' in v)
def snapshot(cs):return sorted([{'id':c['Id'],'image':c['Image'],'started':c['State']['StartedAt'],'mounts':sorted(c['Mounts'],key=lambda x:x['Destination'])} for c in cs],key=lambda x:x['id'])
def quote_ps(value):
    need(not any(c in str(value) for c in ['\r','\n','\x00']),'Windows-path')
    return "'"+str(value).replace("'","''")+"'"
def file_sha_bytes(raw):return hashlib.sha256(raw).hexdigest()
class Runtime:
    def __init__(self,context):
        self.context=private_file(context,0o400);self.cfg=private_json(context)
        self.root=self.context.parent;self.code=Path(__file__).resolve().parent
        self.restored=[];self.targets=[];self.owner_pid=None;self.new_backend=None;self.launcher=None
        self.handoff_delivered=False;self.handoff_released=False;self.lease=None;self.started=time.monotonic();self.pre=None
    def now(self):return time.time()
    def api(self,path):return json.loads(command([GH,'api',REPO+path]))
    def authority(self,existing_session=False):
        c=self.cfg
        need(c.get('enabled') is True,'controller-enabled')
        need(os.getuid()==1000 and os.geteuid()==1000 and stat.S_IMODE(self.root.stat().st_mode)==0o700 and self.root.stat().st_uid==1000,'controller-owner')
        expected_parent=Path.home()/'.local/share/secure-file-drop/operator-acceptance/controller-sessions'
        need(not self.root.is_symlink() and not expected_parent.is_symlink() and stat.S_IMODE(expected_parent.stat().st_mode)==0o700 and self.root.parent.resolve()==expected_parent.resolve() and re.fullmatch(r'sfd-controller-[0-9]{8}T[0-9]{6}Z-[a-f0-9]{12}',self.root.name),'session-root')
        need(self.code.resolve()==(self.root/'controller-code').resolve() and self.context.name=='controller-context.json','sealed-controller-code')
        need(Path(c['live_checkout']).resolve()==(Path.home()/'Secure File Drop').resolve(),'live-checkout-path')
        need(Path(c['windows_session']).resolve()==(Path(c['artifacts']['native_form']).parent/'Sessions'/self.root.name).resolve(),'Windows-session-path')
        profile=command([PS,'-NoProfile','-Command','[Environment]::GetFolderPath("UserProfile")']).decode().strip()
        profile=Path(command(['wslpath','-u',profile]).decode().strip())
        need(Path(c['artifacts']['native_form']).resolve()==(profile/'SecureFileDrop-Staged-Acceptance-20261026-593cd1a8/OwnerStagedInput.exe').resolve(),'Owner-Windows-tool-path')
        need(c.get('enabled') is True and c.get('actual_master')==MASTER and c.get('operator_reference_master')==PROTOCOL_MASTER,'controller-enabled')
        need(0<=self.now()-c['created_epoch'] and (existing_session or self.now()-c['created_epoch']<300) and self.now()<c['expires_epoch']<=c['created_epoch']+3600,'controller-expiry')
        need(not os.environ.get('DOCKER_HOST') and command(['docker','context','inspect','--format','{{.Endpoints.docker.Host}}']).decode().strip()=='unix:///var/run/docker.sock','Docker-target')
        need(re.fullmatch('[a-f0-9]{40}',c['controller_revision']) is not None and
             re.fullmatch('[a-f0-9]{48}',c['controller_nonce']) is not None and
             isinstance(c['controller_review_comment'],int) and c['controller_review_comment']>0 and
             isinstance(c['controller_pr'],int) and c['controller_pr']>0,'controller-revision')
        hashes={p:digest(self.code/p) for p in CODE_PATHS}
        need(canonical(hashes)==c['bundle_sha256'],'controller-bundle')
        tree={v['path']:v for v in self.api('git/trees/'+c['controller_revision']+'?recursive=1')['tree']}
        for p,h in hashes.items():
            import base64
            item=tree['tools/operator-controller/'+p]
            raw=base64.b64decode(self.api('git/blobs/'+item['sha'])['content'])
            need(file_sha_bytes(raw)==h,'published-controller-bytes')
        for ident,h in REVIEWS.items():
            review=self.api('issues/comments/'+str(ident))
            need(review['user']['login']=='dreamingfree09' and file_sha_bytes(review['body'].encode())==h,'frozen-Aether-decision')
        validate_review(self.api('issues/comments/'+str(c['controller_review_comment'])),c)
        self.master_gate()
        checks=self.api('commits/'+c['controller_revision']+'/check-runs?per_page=100')
        need(checks['total_count']==len(checks['check_runs']) and checks['check_runs'] and all(v['status']=='completed' and v['conclusion']=='success' for v in checks['check_runs']),'controller-CI')
        for role in ['scope','prior_plan','current_manifest','baseline','restore_utility','restore_source','image_overlay','retention_overlay','legacy_context']:
            path=Path(c['paths'][role]);need(digest(path)==PINS[role],'pinned-'+role)
        self.plan=json.loads(Path(c['paths']['scope']).read_text());prior=json.loads(Path(c['paths']['prior_plan']).read_text())
        self.base=json.loads(Path(c['paths']['baseline']).read_text());self.legacy=private_json(c['paths']['legacy_context'])
        self.targets=self.plan['missing_restoration_targets'];need(len(self.targets)==3,'three-target-scope')
        self.known={v['name']:v for v in json.loads(Path(c['paths']['current_manifest']).read_text())}
        need(len(self.known)==8,'current-inventory-scope')
        for v in self.targets:need(v['name'] not in self.known,'scope-disjoint');self.known[v['name']]=v
        self.volume=prior['original_backup_volume'];self.old_id=prior['expected_live_backend']
        need(self.legacy['old_backend_id']==self.old_id and self.legacy['master']==PROTOCOL_MASTER,'legacy-identity')
        for role in ['operator_binary','operator_driver','native_form','transport','F196','framework_config','live_acceptance']:
            need(digest(c['artifacts'][role])==PINS[role],'frozen-artifact-'+role)
        live=Path(c['artifacts']['live_acceptance'])
        need(digest(live.with_name('SecureFileDrop.exe'))==PINS['F196'] and digest(live.with_name('OwnerLiveAcceptanceV2.exe.config'))==PINS['framework_config'],'frozen-live-dependencies')
        need(digest(c['artifacts']['probe'])==c['probe_sha256'],'probe-artifact')
        need(Path(c['windows_payload']).stat().st_size==512 and digest(c['windows_payload'])==self.legacy['payload_SHA256'],'frozen-test-payload')
        need(not self.ledger().joinpath('provisioning-started.json').exists(),'one-account-already-reserved')
        need(self.now()+MIN_MARGIN<RENEW_BY,'CRL-renewal-window')
    def review_gate(self):
        for ident,h in REVIEWS.items():
            review=self.api('issues/comments/'+str(ident))
            need(review['user']['login']=='dreamingfree09' and file_sha_bytes(review['body'].encode())==h,'current-Aether-decision')
        validate_review(self.api('issues/comments/'+str(self.cfg['controller_review_comment'])),self.cfg)
    def master_gate(self):
        need(self.api('git/ref/heads/master')['object']['sha']==MASTER,'actual-master-ref')
        commit=self.api('git/commits/'+MASTER)
        need([p['sha'] for p in commit['parents']]==[PROTOCOL_MASTER,OPERATOR_HEAD] and commit['tree']['sha']==OPERATOR_TREE,'actual-master-ancestry')
        for name,ident in MASTER_CI.items():
            r=self.api('actions/runs/'+str(ident))
            need(r['head_sha']==MASTER and r['name']==name and r['status']=='completed' and r['conclusion']=='success','actual-master-CI')
    def ledger(self):
        expected=Path.home()/'.local/share/secure-file-drop/operator-acceptance/work-ledgers'/WORK
        need(Path(self.cfg['work_ledger']).resolve()==expected.resolve(),'work-ledger-path')
        expected.parent.mkdir(parents=True,mode=0o700,exist_ok=True)
        need(not expected.parent.is_symlink() and expected.parent.stat().st_uid==1000 and stat.S_IMODE(expected.parent.stat().st_mode)==0o700,'work-ledger-parent')
        expected.mkdir(mode=0o700,exist_ok=True)
        need(expected.stat().st_uid==1000 and stat.S_IMODE(expected.stat().st_mode)==0o700 and not expected.is_symlink(),'work-ledger-owner')
        return expected
    def acquire_once(self):
        p=self.ledger()/'operation.lock';fd=os.open(p,os.O_CREAT|os.O_RDWR|os.O_NOFOLLOW,0o600)
        need(stat.S_ISREG(os.fstat(fd).st_mode) and os.fstat(fd).st_uid==1000 and not(stat.S_IMODE(os.fstat(fd).st_mode)&0o077),'lease-owner')
        self.lease=os.fdopen(fd,'r+');fcntl.flock(self.lease,fcntl.LOCK_EX|fcntl.LOCK_NB)
        need(not(self.ledger()/'provisioning-started.json').exists(),'one-account-already-reserved')
        new_json(self.root/'controller-started.json',{'context_nonce':self.cfg['controller_nonce'],'epoch':self.now(),'pid':os.getpid(),'context_sha256':digest(self.context)})
    def event(self,name,**fields):new_json(self.root/(name+'-'+secrets.token_hex(4)+'.json'),dict(event=name,epoch=self.now(),**fields))
    def inspect(self,ident):return json.loads(command(['docker','inspect',ident]))[0]
    def services(self):
        ids=command(['docker','ps','-aq','--filter','label=com.docker.compose.project=securefiledrop']).decode().split()
        need(len(ids)==6,'six-services');cs=json.loads(command(['docker','inspect',*ids]))
        need(all(c['State']['Running'] for c in cs),'running-services')
        for name in ['/sfd_postgres','/sfd_minio']:
            item=next(c for c in cs if c['Name']==name)
            need(item['State'].get('Health',{}).get('Status')=='healthy','database-storage-health')
        return cs
    def old(self):
        cs=self.services();need(snapshot(cs)==self.base,'six-service-baseline')
        b=next(c for c in cs if c['Name']=='/sfd_backend');need(b['Id']==self.old_id,'old-backend')
        need(canonical(environment(b))==self.legacy['old_environment_fingerprint'],'existing-environment')
        mount=[m for m in b['Mounts'] if m['Destination']=='/var/backups/sfd']
        need(len(mount)==1 and mount[0]['Type']=='volume' and mount[0]['Name']==self.volume,'backup-volume')
        e=environment(b)
        need(e['SFD_BACKUP_ENABLED']=='true' and e['SFD_BACKUP_INTERVAL']=='24h' and e['SFD_BACKUP_RETENTION_DAYS']=='7' and e.get('SFD_CLEANUP_ENABLED','false')=='false','incumbent-schedules')
        return cs,b
    def scheduler(self,b):
        hz=int(command(['docker','exec',b['Id'],'getconf','CLK_TCK']));need(hz==100,'scheduler-clock')
        raw=command(['docker','exec',b['Id'],'cat','/proc/uptime','/proc/1/stat']).decode().splitlines()
        uptime=float(raw[0].split()[0]);age=uptime-float(raw[1].rsplit(')',1)[1].split()[19])/hz
        remaining=86400-age%86400;need(remaining>=MIN_MARGIN,'scheduler-margin')
        processes=command(['docker','exec',b['Id'],'sh','-c','for p in /proc/[0-9]*/comm; do cat "$p" 2>/dev/null; done']).decode().splitlines()
        need(not any(p in ['pg_dump','gzip','pg_restore'] for p in processes),'backup-job-idle')
        need(self.now()<self.cfg['expires_epoch'] and time.monotonic()-self.started<3600,'session-expiry')
        return remaining,uptime
    def archives(self,b,restored):
        rows=command(['docker','exec',b['Id'],'find','/var/backups/sfd','-mindepth','1','-maxdepth','1','-exec','stat','-c','%n|%F|%s|%Y|%u|%g|%a','{}',';']).decode().splitlines()
        targets={v['name'] for v in self.targets};wanted=(set(self.known)-targets)|set(restored)
        actual={Path(row.rsplit('|',6)[0]).name for row in rows}
        if actual!=wanted:
            self.event('scope-drift',missing_known=sorted(wanted-actual),
                       unexpected_name_hashes=sorted(hashlib.sha256(n.encode()).hexdigest() for n in actual-wanted),
                       live_operation_not_permitted=True)
            raise GateFailure('changed-three-archive-scope')
        seen={}
        for row in rows:
            path,kind,size,mtime,uid,gid,mode=row.rsplit('|',6);name=Path(path).name
            need(kind=='regular file' and name in self.known,'unexpected-backup-entry')
            v=self.known[name]
            need(int(size)==v['bytes'] and int(mtime)==v['mtime'] and command(['docker','exec',b['Id'],'sha256sum',path]).decode().split()[0]==v['SHA256'],'backup-identity')
            need(command(['docker','exec',b['Id'],'gzip','-t',path])==b'','backup-gzip')
            if name in restored:need((uid,gid,mode)==('0','0','600'),'restored-owner-mode')
            seen[name]=(uid,gid,mode)
        targets={v['name'] for v in self.targets};wanted=(set(self.known)-targets)|set(restored)
        need(set(seen)==wanted,'changed-three-archive-scope')
        for v in self.known.values():
            p=private_file(v['private_source'],0o400);s=p.stat()
            need(stat.S_IMODE(p.parent.stat().st_mode)==0o700 and s.st_size==v['bytes'] and int(s.st_mtime)==v['mtime'] and digest(p)==v['SHA256'],'backup-custody')
            gzip.decompress(p.read_bytes())
            need(self.now()+MIN_MARGIN-v['mtime']<30*86400,'30day-maintenance-window')
        return seen
    def sql(self,q,cs=None,target=None,login=None,database=None):
        cs=cs or self.services();pg=next(c for c in cs if c['Name']=='/sfd_postgres');e=environment(pg)
        target=target or pg['Id'];login=login or e['POSTGRES_USER'];database=database or e['POSTGRES_DB']
        q=q.replace('BEGIN READ ONLY;',"BEGIN READ ONLY; SET LOCAL TIME ZONE 'UTC'; SET LOCAL bytea_output='hex'; SET LOCAL statement_timeout='5s'; SET LOCAL lock_timeout='2s';")
        return command(['docker','exec','-i',target,'psql','-X','-qAt','-v','ON_ERROR_STOP=1','-U',login,'-d',database],input=q.encode())
    def rows(self,cs=None,target=None,login=None,database=None):
        kw=dict(cs=cs,target=target,login=login,database=database)
        names=self.sql("BEGIN READ ONLY; SELECT table_name FROM information_schema.tables WHERE table_schema='public' AND table_type='BASE TABLE' ORDER BY table_name; COMMIT;",**kw).decode().splitlines()
        need(names and len(names)<=32 and all(re.fullmatch('[a-z][a-z0-9_]*',n) for n in names),'table-identifiers')
        total=0
        for n in names:
            size=self.sql('BEGIN READ ONLY; SELECT COALESCE(sum(pg_column_size(t)),0) FROM public."'+n+'" t; COMMIT;',**kw)
            total+=int(size.strip());need(total<=32*1024*1024,'bounded-database-snapshot')
        return {n:self.sql('BEGIN READ ONLY; COPY (SELECT to_jsonb(t)::text FROM public."'+n+'" t ORDER BY to_jsonb(t)::text COLLATE "C") TO STDOUT; COMMIT;',**kw).splitlines() for n in names}
    def columns(self,**kw):
        return self.sql("BEGIN READ ONLY; SELECT table_name,column_name,udt_name,is_nullable,COALESCE(column_default,'') FROM information_schema.columns WHERE table_schema='public' ORDER BY table_name,ordinal_position; COMMIT;",**kw)
    def quiet(self,cs,b):
        value=json.loads(self.sql("BEGIN READ ONLY; SELECT json_build_object('active',count(*) FILTER(WHERE state='active'),'idle_tx',count(*) FILTER(WHERE state='idle in transaction'),'locks',count(*) FILTER(WHERE wait_event_type='Lock')) FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid(); COMMIT;",cs))
        need(not any(value.values()),'database-quiescence')
        self.scheduler(b)
    def storage(self,cs,b):
        from urllib.parse import urlsplit
        s=next(c for c in cs if c['Name']=='/sfd_minio');shared=set(b['NetworkSettings']['Networks'])&set(s['NetworkSettings']['Networks']);need(len(shared)==1,'private-storage-network');network=shared.pop()
        e=environment(b);raw=e['SFD_S3_ENDPOINT'];u=urlsplit(raw if '://' in raw else 'http://'+raw)
        aliases=set(s['NetworkSettings']['Networks'][network].get('Aliases') or [])|{s['Name'].lstrip('/'),s['NetworkSettings']['Networks'][network]['IPAddress']}
        need(u.hostname in aliases and not u.username and not u.password,'storage-target')
        values={'Endpoint':raw,'AccessKey':e['SFD_S3_ACCESS_KEY'],'SecretKey':e['SFD_S3_SECRET_KEY'],'Bucket':e['SFD_BUCKET']}
        owner='sfd-controller-storage-'+secrets.token_hex(6);label='sfd.controller-owned'
        args=['docker','run','--rm','-i','--pull=never','--name',owner,'--label',label+'='+owner,'--network',network,'--user','1000:1000','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--pids-limit','64','--memory','256m','--mount','type=bind,source='+self.cfg['artifacts']['probe']+',target=/probe/check,readonly','--entrypoint','/probe/check',IMAGE]
        try:
            result=json.loads(command(args,input=json.dumps(values).encode(),timeout=130));need(result.get('ok') is True,'storage-read-only-snapshot');return result
        finally:
            values.clear()
            p=subprocess.run(['docker','inspect',owner],capture_output=True)
            if p.returncode==0:
                found=json.loads(p.stdout)[0];need(found['Config']['Labels'].get(label)==owner and found['Image']==IMAGE,'owned-storage-probe')
                command(['docker','rm','-f',found['Id']])
    def images(self):
        expected={'backend':PINS['backend'],'sfd-hash':PINS['native_hash']}
        for role,image in [('successor',IMAGE),('artifact-recovery',RECOVERY)]:
            info=json.loads(command(['docker','image','inspect',image]))[0]
            need(info['Id']==image and info['Config']['Labels']['org.opencontainers.image.revision']=='7880ca51ddda4f362e6f9c65d799ba78cf816b2d','image-provenance')
            owner='sfd-controller-image-'+secrets.token_hex(6);label='sfd.controller-owned'
            ident=command(['docker','create','--pull=never','--network','none','--label',label+'='+owner,image]).decode().strip()
            try:
                c=self.inspect(ident);need(c['Config']['Labels'].get(label)==owner and c['Image']==image and not c['State']['Running'] and not c['Mounts'],'owned-image-container')
                d=self.root/owner;d.mkdir(mode=0o700)
                for name,want in expected.items():
                    command(['docker','cp',ident+':/app/'+name,str(d/name)])
                    need(digest(d/name)==want,'image-binary')
            finally:
                c=self.inspect(ident);need(c['Config']['Labels'].get(label)==owner and not c['State']['Running'],'owned-image-cleanup')
                command(['docker','rm',ident])
    def backup(self,cs):
        need(int(self.sql('BEGIN READ ONLY; SELECT pg_database_size(current_database()); COMMIT;',cs).strip())<=128*1024*1024,'bounded-recovery-database')
        before=self.rows(cs);columns=self.columns(cs=cs)
        pg=next(c for c in cs if c['Name']=='/sfd_postgres');e=environment(pg)
        version=command(['docker','exec',pg['Id'],'pg_dump','--version']).decode()
        need('PostgreSQL) 16.' in version,'pg-dump-version')
        args=['docker','exec','-e','PGOPTIONS=-c default_transaction_read_only=on -c statement_timeout=60000',
              pg['Id'],'pg_dump','--format=custom','--no-owner','--no-acl','--no-password',
              '--lock-wait-timeout=5s','-U',e['POSTGRES_USER'],'-d',e['POSTGRES_DB']]
        raw=command(args,timeout=90);need(0<len(raw)<=128*1024*1024,'private-backup-bytes')
        p=self.root/'private-live-schema11.dump'
        fd=os.open(p,os.O_CREAT|os.O_EXCL|os.O_WRONLY|os.O_NOFOLLOW,0o600)
        with os.fdopen(fd,'wb') as out:out.write(raw);out.flush();os.fsync(out.fileno())
        p.chmod(0o400)
        need(self.rows(cs)==before and self.columns(cs=cs)==columns,'backup-source-preservation')
        owner='sfd-controller-restore-'+secrets.token_hex(6);label='sfd.controller-owned';volume=owner+'-data';cid=None;ok=False
        need(not command(['docker','volume','ls','-q','--filter','name=^'+volume+'$']).strip(),'fixture-volume-new')
        command(['docker','volume','create','--label',label+'='+owner,volume])
        db='sfd_restore_'+secrets.token_hex(6);runenv=dict(os.environ,POSTGRES_PASSWORD=secrets.token_hex(24))
        try:
            cid=command(['docker','run','-d','--pull=never','--name',owner,'--label',label+'='+owner,'--network','none',
                         '-e','POSTGRES_USER=fixture','-e','POSTGRES_DB='+db,'-e','POSTGRES_PASSWORD',
                         '--mount','type=volume,source='+volume+',target=/var/lib/postgresql/data',PGIMAGE],env=runenv).decode().strip()
            c=self.inspect(cid);need(c['Image']==PGIMAGE and c['Config']['Labels'].get(label)==owner and c['HostConfig']['NetworkMode']=='none' and not c['HostConfig']['PortBindings'],'restore-fixture-identity')
            for _ in range(120):
                ready=subprocess.run(['docker','exec',cid,'pg_isready','-h','127.0.0.1','-U','fixture','-d',db],capture_output=True)
                if ready.returncode==0:break
                time.sleep(.25)
            need(ready.returncode==0,'restore-fixture-ready')
            command(['docker','exec','-i',cid,'pg_restore','--exit-on-error','--no-owner','--no-acl','-U','fixture','--dbname',db],input=raw,timeout=90)
            restored=self.rows(cs,cid,'fixture',db)
            need(restored==before and self.columns(cs=cs,target=cid,login='fixture',database=db)==columns,'private-restore-contents')
            need(self.rows(cs)==before,'private-restore-source-unchanged');ok=True
        finally:
            if cid:
                c=self.inspect(cid);need(c['Image']==PGIMAGE and c['Config']['Labels'].get(label)==owner,'restore-fixture-ownership')
                if ok:command(['docker','rm','-f',cid])
                else:command(['docker','stop','--time','10',cid],timeout=20)
            v=json.loads(command(['docker','volume','inspect',volume]))[0]
            need(v['Labels'].get(label)==owner,'restore-volume-ownership')
            if ok:command(['docker','volume','rm',volume])
            self.event('private-restore-proof',success=ok,archive_sha256=file_sha_bytes(raw),archive_bytes=len(raw),
                       fresh=True,network_none=True,original_data_unchanged=ok,failure_fixture_retained=not ok)
        return {'rows':before,'columns':columns,'backup_epoch':self.now(),'backup_sha256':file_sha_bytes(raw)}
    def compose(self,cs,b):
        c=self.cfg;labels=b['Config']['Labels']
        files=[Path(p) for p in labels['com.docker.compose.project.config_files'].split(',')]
        need(len(files)==2 and all(p.is_file() for p in files),'compose-source-files')
        project=Path(c['live_checkout'])
        need(command(['git','-C',str(project),'branch','--show-current']).decode().strip()=='feature/v2-enhancements' and
             command(['git','-C',str(project),'rev-parse','HEAD']).decode().strip()=='2b8e56de4b79e2508125ca02c18455ae88dee7f6' and
             not command(['git','-C',str(project),'status','--porcelain']).strip(),'preserved-feature-checkout')
        need(file_sha_bytes((project/'.env').read_bytes())==file_sha_bytes(private_file(c['validated_environment']).read_bytes()),'validated-env-bytes')
        args=['docker','compose','-p','securefiledrop','--project-directory',str(project)]
        for p in files:args+=['-f',str(p)]
        extra=['-f',c['paths']['image_overlay'],'-f',c['paths']['retention_overlay']]
        before=json.loads(command(args+['config','--format','json']))
        after=json.loads(command(args+extra+['config','--format','json']))
        need(set(before['services'])==set(after['services']) and len(before['services'])==6,'compose-service-set')
        for n in before['services']:
            if n!='backend':need(before['services'][n]==after['services'][n],'other-service-configurations')
        a=before['services']['backend'];z=after['services']['backend']
        need({k for k in set(a)|set(z) if a.get(k)!=z.get(k)}=={'image','environment','stop_grace_period'},'only-three-backend-differences')
        need(z['image']==IMAGE and z['stop_grace_period']=='25s','accepted-image-stop-grace')
        need({k for k in set(a['environment'])|set(z['environment']) if a['environment'].get(k)!=z['environment'].get(k)}=={'SFD_BACKUP_RETENTION_DAYS'},'only-retention-env-difference')
        actual=environment(b);image=json.loads(command(['docker','image','inspect',IMAGE]))[0]
        desired=environment(image);desired.update({k:str(v).replace('$$','$') for k,v in z['environment'].items() if v is not None})
        need(desired==dict(actual,SFD_BACKUP_RETENTION_DAYS='30'),'exact-effective-secrets')
        allowed={'build','container_name','depends_on','environment','image','labels','networks','ports','restart','volumes','stop_grace_period','entrypoint','command','working_dir','expose','healthcheck','user','read_only','privileged','cap_add','cap_drop','security_opt'}
        need(set(z)<=allowed,'unexpected-compose-option')
        host=b['HostConfig'];need(z.get('privileged',False)==host['Privileged'] and z.get('read_only',False)==host['ReadonlyRootfs'],'runtime-security-options')
        for ck,hk in [('cap_add','CapAdd'),('cap_drop','CapDrop'),('security_opt','SecurityOpt')]:
            need((z.get(ck) or [])==(host.get(hk) or []),'runtime-capabilities')
        ports={}
        for p in z.get('ports',[]):
            need(p.get('mode','ingress')=='ingress' and str(p['published']).isdigit(),'compose-port-form')
            ports.setdefault(str(p['target'])+'/'+p.get('protocol','tcp'),[]).append({'HostIp':p.get('host_ip',''),'HostPort':str(p['published'])})
        need(ports==(host.get('PortBindings') or {}),'no-new-host-ports')
        nets={after['networks'][n]['name'] for n in z.get('networks',{})}
        need(nets==set(b['NetworkSettings']['Networks']),'same-backend-networks')
        old_mounts={(m['Type'],m.get('Name') if m['Type']=='volume' else m['Source'],m['Destination'],m['RW']) for m in b['Mounts']}
        mounts=set()
        for v in z.get('volumes',[]):
            source=after['volumes'][v['source']]['name'] if v['type']=='volume' else v['source']
            mounts.add((v['type'],source,v['target'],not v.get('read_only',False)))
        need(mounts==old_mounts,'same-backend-mounts')
        need((z.get('entrypoint') or image['Config'].get('Entrypoint'))==b['Config'].get('Entrypoint') and
             (z.get('command') or image['Config'].get('Cmd'))==b['Config'].get('Cmd'),'same-entrypoint-command')
        return args+extra,desired
    def tls(self):
        need(self.now()+MIN_MARGIN<RENEW_BY,'CRL-renewal-window')
        root=Path(self.cfg['paths']['public_ca']);crl=Path(self.cfg['paths']['public_crl'])
        need(root.name=='root-ca.crt' and crl.name=='sfd-local-ca.crl' and digest(crl)==PINS['CRL'],'public-TLS-material')
        der=command(['openssl','x509','-in',str(root),'-outform','DER'])
        need(file_sha_bytes(der)=='0f45e72366dc8ff6e3bed9323ef697bb44b21204d7e05ea303da319b5160bc28','existing-root')
        command(['openssl','crl','-inform','DER','-in',str(crl),'-noout','-verify','-CAfile',str(root)])
        info=command(['openssl','crl','-inform','DER','-in',str(crl),'-noout','-crlnumber','-nextupdate']).decode()
        need('crlNumber=0x1001' in info and 'Oct 29 18:10:29 2026 GMT' in info,'current-CRL')
        index=Path(self.cfg['paths']['issuer_index']);idx=index.lstat()
        need(stat.S_ISREG(idx.st_mode) and idx.st_uid==1000 and not(stat.S_IMODE(idx.st_mode)&0o022),'issuer-ledger-owner')
        rows=[s.split('\t') for s in index.read_text().splitlines() if s]
        need(collections.Counter(s[0] for s in rows)=={'V':2},'issuer-ledger')
        counter=Path(self.cfg['paths']['crl_counter']);ct=counter.lstat()
        need(stat.S_ISREG(ct.st_mode) and ct.st_uid==1000 and not(stat.S_IMODE(ct.st_mode)&0o022) and counter.read_text().strip()=='1002','CRL-counter')
        out=self.root/('TLS-'+secrets.token_hex(4)+'.json')
        local_script=Path(self.cfg['windows_session'])/'strict_tls.ps1'
        need(digest(local_script)==digest(self.code/'strict_tls.ps1'),'pinned-local-Windows-TLS-script')
        src=command(['wslpath','-w',str(local_script)]).decode().strip()
        dest=command(['wslpath','-w',str(out)]).decode().strip()
        command([PS,'-NoProfile','-File',src,'-ReportPath',dest],timeout=45)
        v=json.loads(out.read_text());need(v.get('success') is True,'strict-Windows-TLS')
        self.tls_epoch=self.now()
    def mail(self,b):
        e=environment(b);need(e['SFD_SMTP_HOST']=='smtp.gmail.com' and e['SFD_SMTP_USER'].casefold()==self.cfg['controlled_mailbox'].casefold() and e['SFD_SMTP_FROM'].casefold()==self.cfg['controlled_mailbox'].casefold(),'controlled-mailbox')
        with smtplib.SMTP(e['SFD_SMTP_HOST'],int(e.get('SFD_SMTP_PORT','587')),timeout=12) as s:
            s.ehlo();need(s.has_extn('starttls'),'SMTP-STARTTLS');s.starttls(context=ssl.create_default_context());s.ehlo();s.login(e['SFD_SMTP_USER'],e['SFD_SMTP_PASS'])
        with imaplib.IMAP4_SSL('imap.gmail.com',993,ssl_context=ssl.create_default_context(),timeout=12) as m:
            m.login(e['SFD_SMTP_USER'],e['SFD_SMTP_PASS'])
        self.event('controlled-mailbox-authenticated',mail_sent=False,unrelated_messages_read=False)
    def traffic(self,cs,b,expected):
        samples=[];previous=None
        for _ in range(3):
            self.quiet(cs,b)
            current=self.rows(cs);need(current==expected,'business-rows-quiescence')
            raw=command(['docker','exec',b['Id'],'cat','/proc/net/tcp','/proc/net/tcp6']).decode().splitlines()
            queues=[]
            for line in raw:
                f=line.split()
                if len(f)>4 and f[1].upper().endswith(':1F90') and f[3]=='01':
                    q=[int(v,16) for v in f[4].split(':')];need(not any(q),'inbound-queue-activity');queues.append(q)
            samples.append(len(queues))
            remaining,up=self.scheduler(b);mono=time.monotonic()
            if previous:
                need(.9<(up-previous[0])/(mono-previous[1])<1.1,'stable-container-clock')
            previous=(up,mono);time.sleep(5)
        self.event('bounded-activity',samples=len(samples),inbound_connections=samples,queues_zero=True,
                   database_rows_stable=True,Owner_IDLE_separate=True,exclusive_future_traffic_not_claimed=True)
        return remaining
    def preflight(self):
        self.master_gate();cs,b=self.old();archive_metadata=self.archives(b,[]);self.scheduler(b)
        need(self.sql('BEGIN READ ONLY; SELECT version,dirty FROM schema_migrations; COMMIT;',cs).decode().strip()=='11|f','clean-schema11')
        role=self.sql("BEGIN READ ONLY; SELECT count(*)=1 AND bool_and(data_type='boolean' AND is_nullable='NO' AND column_default='false') FROM information_schema.columns WHERE table_schema='public' AND table_name='users' AND column_name='is_admin'; COMMIT;",cs)
        need(role.strip()==b't','original-admin-column')
        layout=self.sql("BEGIN READ ONLY; SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='files' AND column_name='sha256_bytes'; SELECT data_type FROM information_schema.columns WHERE table_schema='public' AND table_name='files' AND column_name='user_id'; COMMIT;",cs).decode().splitlines()
        need(layout==['bytea','uuid'],'unchanged-live-BYTEA-ownership-layout')
        integrity=self.sql("BEGIN READ ONLY; SELECT count(*) FROM files f LEFT JOIN users u ON u.id=f.user_id WHERE (f.user_id IS NOT NULL AND u.id IS NULL) OR f.size_bytes<0 OR f.status NOT IN ('pending','stored','hashed','ready','failed') OR (f.sha256_bytes IS NOT NULL AND octet_length(f.sha256_bytes)<>32) OR (f.sha256_hex IS NOT NULL AND f.sha256_hex !~ '^[0-9a-f]{64}$') OR (f.sha256_bytes IS NOT NULL AND f.sha256_hex IS NOT NULL AND encode(f.sha256_bytes,'hex')<>trim(f.sha256_hex)) OR (f.status IN ('hashed','ready') AND (f.sha256_hex IS NULL OR f.sha256_bytes IS NULL)); COMMIT;",cs).strip()
        need(integrity==b'0','existing-file-owner-hash-integrity')
        self.quiet(cs,b);self.compose(cs,b);self.images();objects=self.storage(cs,b);private=self.backup(cs)
        self.tls();self.mail(b);remaining=self.traffic(cs,b,private['rows'])
        snapshot_preserved(objects,self.storage(cs,b))
        self.pre=dict(private,objects=objects,old_backend=b,services=cs,scheduler_margin=remaining,archive_metadata=archive_metadata)
        self.event('all-initial-gates',passed=True,scheduler_margin_seconds=remaining,
                   source_schema=11,private_restore_verified=True,object_count=len(objects['objects']))
        return self.pre
    def check_owner(self,ready):
        validate_ready(ready,self.now())
        presence=private_json(self.root/'owner-present.json')
        need(presence['Windows_PID']==ready['Windows_PID'] and presence['controller_nonce']==self.cfg['controller_nonce'],'Owner-presence-binding')
        self.window_alive(ready['Windows_PID'])
        pid=ready['pid'];proc=Path('/proc')/str(pid)
        need(proc.stat().st_uid==1000 and (proc/'cmdline').read_bytes().split(b'\x00').count(str(self.code/'staged_driver.py').encode())==1,'frozen-driver-process')
        status=(proc/'status').read_text()
        parent=int(next(s.split()[1] for s in status.splitlines() if s.startswith('PPid:')))
        need(parent==presence['pid'],'broker-driver-parent')
    def window_alive(self,pid):
        need(isinstance(pid,int) and pid>1,'native-PID')
        script="$ErrorActionPreference='Stop';$p=Get-Process -Id "+str(pid)+";if($p.MainWindowHandle -eq 0 -or $p.SessionId -ne (Get-Process -Id $PID).SessionId){exit 3};if((Get-FileHash -LiteralPath $p.Path -Algorithm SHA256).Hash.ToLowerInvariant() -cne '"+PINS['native_form']+"'){exit 4}"
        command([PS,'-NoProfile','-Command',script])
    def prepare_input(self,pre):
        c=self.cfg;dest=self.code/'staged_driver.py'
        need(digest(dest)==c['controller_driver_sha256'],'controller-owned-driver')
        broker={'execution_authorised':True,'actual_master':MASTER,'state_directory':str(self.root),
                'operator_driver':str(dest),'operator_context':str(self.root/'operator-context-ready.json'),
                'gates_receipt':str(self.root/'operator-gates.json'),'controller_nonce':c['controller_nonce'],
                'controller_context':str(self.context),'controller_context_sha256':digest(self.context)}
        new_json(self.root/'broker-context.json',broker)
        broker['operator_context']=str(self.root/'operator-context-ready.json')
        winroot=Path(c['windows_session'])
        need(winroot.is_dir() and not(winroot/'Windows-input-context.json').exists(),'new-Windows-session')
        broker_source=self.code/'broker.py'
        config={'execution_authorised':True,'Windows_driver':command(['wslpath','-w',str(broker_source)]).decode().strip(),
                'driver_sha256':digest(broker_source),'Wsl_driver':str(broker_source),
                'Wsl_context':str(self.root/'broker-context.json')}
        # This configuration contains no credential; the private WSL context remains0400.
        new_json(winroot/'Windows-input-context.json',config)
        exe=command(['wslpath','-w',c['artifacts']['native_form']]).decode().strip()
        path=command(['wslpath','-w',str(winroot/'Windows-input-context.json')]).decode().strip()
        script="$ErrorActionPreference='Stop';$p=Start-Process -FilePath "+quote_ps(exe)+" -ArgumentList "+quote_ps('"'+path+'"')+" -WindowStyle Normal -PassThru;[Console]::WriteLine($p.Id);$p.WaitForExit()"
        self.launcher=subprocess.Popen([PS,'-NoProfile','-Command',script],stdout=subprocess.PIPE,stderr=subprocess.DEVNULL)
        self.owner_pid=int(self.launcher.stdout.readline(32));self.window_alive(self.owner_pid)
        self.event('native-form-launched',pid=self.owner_pid,one_existing_admin_password_only=True)
    def wait_owner(self):
        end=time.monotonic()+1200
        while time.monotonic()<end:
            self.window_alive(self.owner_pid)
            need(not(self.root/'broker-stopped.json').exists(),'broker-authority-stopped')
            if (self.root/'owner-present.json').exists():break
            need(self.now()<self.cfg['expires_epoch'],'Owner-wait-expiry');time.sleep(1)
        else:raise GateFailure('Owner-input-timeout')
        presence=private_json(self.root/'owner-present.json')
        need(presence['Windows_PID']==self.owner_pid and presence['idle'] is True and presence['controller_nonce']==self.cfg['controller_nonce'],'actual-Owner-presence')
        bp=Path('/proc')/str(presence['pid'])
        need(bp.stat().st_uid==1000 and (bp/'cmdline').read_bytes().split(b'\x00').count(str(self.code/'broker.py').encode())==1,'broker-process')
        # The broker holds admin input privately while these gates refresh.
        self.recheck(self.pre,[],None,deep=True);self.window_alive(self.owner_pid)
        username='sfd_acceptance_20261026'
        for _ in range(5):
            exists=self.sql("BEGIN READ ONLY; SELECT EXISTS(SELECT 1 FROM users WHERE username='"+username+"'); COMMIT;").strip()
            if exists==b'f':break
            username='sfd_acceptance_20261026_'+secrets.token_hex(4)
        need(exists==b'f','available-labelled-identity')
        gates={'UTC':datetime.datetime.now(datetime.timezone.utc).isoformat(),'all_mandatory_gates_passed':True,
               'reviewed_three_original_scope':True,'scheduler_margin_seconds':self.scheduler(self.old()[1])[0],
               'Owner_credentials_requested':True,'Owner_input_in_private_broker':True,'available_acceptance_username':username,
               'actual_master':MASTER,'controller_nonce':self.cfg['controller_nonce']}
        new_json(self.root/'operator-gates.json',gates)
        artifacts={self.cfg['artifacts'][r]:PINS[r] for r in ['operator_binary','operator_driver','native_form','transport','F196','framework_config','live_acceptance']}
        live=Path(self.cfg['artifacts']['live_acceptance'])
        artifacts[str(live.with_name('SecureFileDrop.exe'))]=PINS['F196']
        artifacts[str(live.with_name('OwnerLiveAcceptanceV2.exe.config'))]=PINS['framework_config']
        c={'master':MASTER,'actual_master':MASTER,'candidate':IMAGE,'execution_authorised':True,
           'independent_review':'https://github.com/dreamingfree09/secure-file-drop/pull/37#issuecomment-6427391643',
           'state_directory':str(self.root),'driver_sha256':self.cfg['controller_driver_sha256'],'operator_reference_head':OPERATOR_HEAD,
           'controller_context':str(self.context),'controller_context_sha256':digest(self.context),
           'controller_revision':self.cfg['controller_revision'],'controller_nonce':self.cfg['controller_nonce'],
           'operator_binary':self.cfg['artifacts']['operator_binary'],'strict_transport':self.cfg['artifacts']['transport'],
           'release':self.cfg['artifacts']['F196'],'native_form_sha256':PINS['native_form'],'artifacts':artifacts,
           'gates_receipt':str(self.root/'operator-gates.json'),'gates_sha256':digest(self.root/'operator-gates.json'),
           'old_backend_id':self.old_id,'initial_service_ids':self.legacy['initial_service_ids'],
           'controlled_mailbox':self.cfg['controlled_mailbox'],'live_acceptance':self.cfg['artifacts']['live_acceptance'],
           'Windows_payload':self.cfg['Windows_payload']}
        new_json(self.root/'operator-context-ready.json',c)
        new_json(self.root/'operator-ready.json',{'controller_nonce':self.cfg['controller_nonce'],'context_sha256':digest(self.root/'operator-context-ready.json')})
        end=time.monotonic()+45
        while time.monotonic()<end:
            self.window_alive(self.owner_pid)
            if (self.root/'owner-ready.json').exists():return private_json(self.root/'owner-ready.json')
            need(not(self.root/'broker-stopped.json').exists(),'private-administrator-check');time.sleep(.5)
        raise GateFailure('administrator-readiness-timeout')
    def recheck(self,pre,restored,ready,deep=False):
        self.review_gate();self.master_gate();cs,b=self.old();self.scheduler(b);current=self.archives(b,restored)
        need(all(current.get(n)==v for n,v in pre['archive_metadata'].items()),'current-backup-metadata-preserved')
        if ready:self.check_owner(ready)
        need(self.now()-pre['backup_epoch']<1800,'fresh-private-restore')
        need(self.rows(cs)==pre['rows'] and self.columns(cs=cs)==pre['columns'],'pre-adoption-rows')
        self.quiet(cs,b);self.compose(cs,b)
        if deep:
            self.tls();snapshot_preserved(pre['objects'],self.storage(cs,b));self.traffic(cs,b,pre['rows'])
        if self.now()-self.tls_epoch>=120:self.tls();self.quiet(cs,b)
        need(self.now()-self.tls_epoch<180,'fresh-strict-TLS')
    def restore_one(self,spec):
        cs,b=self.old();name=spec['name'];need(name in {v['name'] for v in self.targets} and name not in self.restored,'one-allowed-archive')
        source=private_file(spec['private_source'],0o400);need(digest(source)==spec['SHA256'],'restore-source')
        owner='sfd-controller-publish-'+secrets.token_hex(6);label='sfd.controller-owned'
        args=['docker','run','--rm','-i','--pull=never','--name',owner,'--label',label+'='+owner,'--network','none','--user','0:0',
              '--read-only','--cap-drop','ALL','--security-opt','no-new-privileges','--pids-limit','64','--memory','128m',
              '--mount','type=bind,source='+self.cfg['paths']['restore_utility']+',target=/probe/recover,readonly',
              '--mount','type=volume,source='+self.volume+',target=/target','--entrypoint','/probe/recover',IMAGE,
              '-directory','/target','-archive',name]
        ok=False
        try:
            p=subprocess.run(args,input=source.read_bytes(),capture_output=True,timeout=30)
            need(p.returncode==0,'exclusive-archive-publication');v=json.loads(p.stdout)
            need(v.get('success') and v.get('SHA256')==spec['SHA256'] and v.get('mtime')==spec['mtime'] and v.get('uid')==0 and v.get('gid')==0 and v.get('file_and_directory_fsync') and v.get('exclusive_same_filesystem_publication'),'durable-archive-publication')
            self.restored.append(name);self.archives(b,self.restored);ok=True
        finally:
            found=subprocess.run(['docker','inspect',owner],capture_output=True)
            if found.returncode==0:
                value=json.loads(found.stdout)[0];need(value['Config']['Labels'].get(label)==owner and value['Image']==IMAGE,'publication-container-owner')
                command(['docker','rm','-f',value['Id']])
            # Observed occupancy is retained even if utility acknowledgement/fsync failed.
            present=subprocess.run(['docker','exec',b['Id'],'test','-e','/var/backups/sfd/'+name],capture_output=True).returncode==0
            self.event('archive-attempt',target=name,acknowledged_success=ok,observed_present=present,
                       group_atomic=False,automatic_rollback_or_deletion=False)
    def stop_old(self):
        cs,b=self.old();self.check_owner(private_json(self.root/'owner-ready.json'))
        self.recheck(self.pre,self.restored,private_json(self.root/'owner-ready.json'))
        self.compose_args,self.expected_env=self.compose(cs,b)
        command(['docker','stop','--time','25',b['Id']],timeout=40)
        state=self.inspect(b['Id'])['State']
        need(not state['Running'] and not state['OOMKilled'] and state['ExitCode']==0,'old-clean-shutdown')
    def adopt(self):
        self.adopt_epoch=self.now()
        command(self.compose_args+['up','-d','--no-deps','--no-build','--pull','never','--force-recreate','backend'],timeout=90)
        cs=self.services();b=next(c for c in cs if c['Name']=='/sfd_backend');self.new_backend=b
        need(b['Id']!=self.old_id and b['Image']==IMAGE and b['Config'].get('StopTimeout')==25,'adopted-image')
        need(environment(b)==self.expected_env and b['Mounts']==self.pre['old_backend']['Mounts'] and
             set(b['NetworkSettings']['Networks'])==set(self.pre['old_backend']['NetworkSettings']['Networks']),'adopted-config')
        self.other_services(cs)
    def other_services(self,cs):
        old=[c for c in self.base if c['id']!=self.old_id]
        b=next(c for c in cs if c['Name']=='/sfd_backend')
        now=[c for c in snapshot(cs) if c['id']!=b['Id']]
        need(now==old,'other-five-preserved')
        if self.pre:
            prior={c['Id']:c for c in self.pre['services'] if c['Id']!=self.old_id}
            for item in cs:
                if item['Id']==b['Id']:continue
                reference=prior[item['Id']]
                need(item['Config']==reference['Config'] and item['HostConfig']==reference['HostConfig'] and
                     item['NetworkSettings']['Networks']==reference['NetworkSettings']['Networks'],'other-five-config-networks')
    def startup_archives(self,b):
        paths=command(['docker','exec',b['Id'],'find','/var/backups/sfd','-mindepth','1','-maxdepth','1','-exec','stat','-c','%n|%F|%s|%Y|%u|%g|%a','{}',';']).decode().splitlines()
        seen={};extra=[]
        for row in paths:
            path,kind,size,mtime,uid,gid,mode=row.rsplit('|',6);name=Path(path).name;need(kind=='regular file','startup-backup-type')
            h=command(['docker','exec',b['Id'],'sha256sum',path]).decode().split()[0]
            if name in self.known:
                v=self.known[name]
                if name in {t['name'] for t in self.targets}:need((uid,gid,mode)==('0','0','600'),'restored-metadata-preserved')
                else:need((uid,gid,mode)==self.pre['archive_metadata'][name],'old-backup-metadata-preserved')
                need(int(size)==v['bytes'] and int(mtime)==v['mtime'] and h==v['SHA256'],'all-eleven-backups-preserved');seen[name]=h
            else:
                need(re.fullmatch(r'sfd-backup-[0-9]{8}-[0-9]{6}\.sql\.gz',name) and int(size)>0 and self.adopt_epoch-5<=int(mtime)<=self.now()+5,'legitimate-startup-backup')
                extra.append((path,h,int(size)))
        need(set(seen)==set(self.known) and len(extra)==1,'startup-backup-count')
        raw=command(['docker','exec',b['Id'],'cat',extra[0][0]]);gzip.decompress(raw)
        p=self.root/'retained-startup-backup.sql.gz'
        if not p.exists():
            fd=os.open(p,os.O_CREAT|os.O_EXCL|os.O_WRONLY|os.O_NOFOLLOW,0o400)
            with os.fdopen(fd,'wb') as f:f.write(raw);f.flush();os.fsync(f.fileno())
        need(digest(p)==extra[0][1],'startup-custody-copy')
        return 12
    def verify_adopted(self,pre):
        cs=self.services();b=next(c for c in cs if c['Name']=='/sfd_backend');self.other_services(cs)
        for _ in range(60):
            ledger=self.sql('BEGIN READ ONLY; SELECT version,dirty FROM schema_migrations; COMMIT;',cs).strip()
            if ledger==b'12|f':break
            time.sleep(.5)
        need(ledger==b'12|f','clean-additive-schema12')
        after=self.rows(cs);need(set(after)==set(pre['rows'])|{'auth_sessions'},'only-session-table-added')
        original_rows_preserved(pre['rows'],after,False)
        old_columns=[line for line in self.columns(cs=cs).splitlines() if not line.startswith(b'auth_sessions|')]
        need(old_columns==pre['columns'].splitlines(),'additive-column-metadata-only')
        session_contract=self.sql("BEGIN READ ONLY; SELECT count(*) FROM information_schema.columns WHERE table_schema='public' AND table_name='auth_sessions' AND is_nullable='NO' AND ((column_name='token_hash' AND data_type='text') OR (column_name='expires_at' AND data_type='timestamp with time zone')); COMMIT;",cs).strip()
        need(session_contract==b'2','session-table-contract')
        snapshot_preserved(pre['objects'],self.storage(cs,b))
        for _ in range(120):
            procs=command(['docker','exec',b['Id'],'sh','-c','for p in /proc/[0-9]*/comm; do cat "$p" 2>/dev/null; done']).decode().splitlines()
            if not any(p in ['pg_dump','gzip'] for p in procs):break
            time.sleep(.5)
        need(not any(p in ['pg_dump','gzip'] for p in procs),'startup-backup-finished')
        self.startup_archives(b);self.tls()
        self.event('adopted-before-account',actual_master=MASTER,operator_reference_master=PROTOCOL_MASTER,
                   schema=12,image=IMAGE,other_five_unchanged=True,all_eleven_plus_startup_backup=True)
    def reserve_account_once(self):
        new_json(self.ledger()/'provisioning-started.json',{'actual_master':MASTER,'context_nonce':self.cfg['controller_nonce'],
                 'epoch':self.now(),'scope':'one normal SMTP-verified ordinary account; no automatic retry/reset/delete'})
    def trigger_operator(self,ready):
        self.check_owner(ready)
        new_json(self.root/'backend-adopted.json',{'nonce':ready['nonce'],'master':MASTER,'actual_master':MASTER,
                 'candidate':IMAGE,'schema':12,'backend_id':self.new_backend['Id']})
    def wait_result(self):
        end=time.monotonic()+360
        while time.monotonic()<end:
            self.window_alive(self.owner_pid)
            if (self.root/'result.json').exists():return private_json(self.root/'result.json')
            need(not(self.root/'broker-stopped.json').exists(),'private-operator-stopped');time.sleep(.5)
        raise GateFailure('post-adoption-operator-timeout')
    def verify_final(self,pre,result):
        cs=self.services();b=next(c for c in cs if c['Name']=='/sfd_backend')
        need(b['Id']==self.new_backend['Id'] and b['Image']==IMAGE and environment(b)==self.expected_env,'final-backend')
        self.other_services(cs);self.startup_archives(b)
        original_rows_preserved(pre['rows'],self.rows(cs),True)
        ident=str(uuid.UUID(result['account_id']));file_id=str(uuid.UUID(result['new_acceptance_file_id']))
        need(ident==result['account_id'] and re.fullmatch(r'sfd_acceptance_20261026(?:_[a-f0-9]{8})?',result['username']),'new-account-result')
        q="BEGIN READ ONLY; SELECT json_build_object('ordinary',NOT is_admin,'active',is_active,'verified',email_verified,'quota',storage_quota_bytes,'bcrypt12',password_hash LIKE '$2_%$12$%','files',(SELECT count(*) FROM files WHERE user_id=u.id),'receipt',(SELECT count(*) FROM audit_logs WHERE resource_id=u.id::text AND metadata->>'work_id'='"+WORK+"')) FROM users u WHERE id='"+ident+"'; COMMIT;"
        account=json.loads(self.sql(q,cs))
        need(account=={'ordinary':True,'active':True,'verified':True,'quota':1048576,'bcrypt12':True,'files':1,'receipt':1},'real-account-quota-audit')
        row=json.loads(self.sql("BEGIN READ ONLY; SELECT json_build_object('owner',user_id::text,'bytes',size_bytes,'hash',trim(sha256_hex),'status',status) FROM files WHERE id='"+file_id+"'; COMMIT;",cs))
        need(row['owner']==ident and row['bytes']==512 and row['hash']==result['payload_sha256'] and row['status'] in ['hashed','ready'],'real-owned-file')
        objects=self.storage(cs,b);snapshot_preserved(pre['objects'],objects,{'sha256':result['payload_sha256']})
        need(sum(len(v) for k,v in self.rows(cs).items() if k=='users')==len(pre['rows']['users'])+1,'exactly-one-new-user')
        need(len(self.rows(cs)['files'])==len(pre['rows']['files'])+1,'exactly-one-new-file')
        self.tls()
        self.event('final-acceptance',username=result['username'],account_id=ident,new_file_id=file_id,
                   quota_bytes=1048576,old_data_and_objects_preserved=True,Owner_visual_acceptance=False)
    def release_handoff(self,ready,result):
        self.window_alive(self.owner_pid)
        self.handoff_released=True
        new_json(self.root/'handoff-approved.json',{'success':True,'nonce':ready['nonce'],'actual_master':MASTER,'epoch':self.now()})
        end=time.monotonic()+15
        while time.monotonic()<end:
            if (self.root/'handoff-delivered.json').exists():
                v=private_json(self.root/'handoff-delivered.json');need(v.get('success') and v['username']==result['username'],'private-handoff-delivered');self.handoff_delivered=True;self.pre.clear();self.expected_env.clear();self.new_backend=None;return
            time.sleep(.25)
        raise GateFailure('private-handoff-timeout')
    def fail_closed_backend(self):
        # Never restart the legacy image or downgrade a database after migration.
        ids=command(['docker','ps','-aq','--filter','label=com.docker.compose.project=securefiledrop','--filter','label=com.docker.compose.service=backend']).decode().split()
        for ident in ids:
            c=self.inspect(ident)
            if c['Id']!=self.old_id and c['Image']==IMAGE:
                command(['docker','stop','--time','25',ident],timeout=40)
    def abort_handoff(self,stage):
        if not(self.root/'abort.json').exists():new_json(self.root/'abort.json',{'stage':stage,'epoch':self.now(),'password_may_have_been_disclosed':self.handoff_released})
        if self.owner_pid:
            try:
                self.window_alive(self.owner_pid)
                command([PS,'-NoProfile','-Command',"$p=Get-Process -Id "+str(self.owner_pid)+";$null=$p.CloseMainWindow()"])
            except Exception:pass
    def finish(self):
        if self.lease:self.lease.close()
        # Keep the verified native parent alive for local credential handoff.
        # No further backend/data mutation is permitted during this wait.
        if self.handoff_delivered and self.launcher is not None:
            print('PRIVATE ACCOUNT READY — credentials only in the local Windows interface.',flush=True)
            end=min(time.monotonic()+905,time.monotonic()+max(0,self.cfg['expires_epoch']-self.now()))
            while self.launcher.poll() is None and time.monotonic()<end:time.sleep(1)
            if self.launcher.poll() is None:
                try:
                    self.window_alive(self.owner_pid)
                    command([PS,'-NoProfile','-Command',"$p=Get-Process -Id "+str(self.owner_pid)+";$null=$p.CloseMainWindow()"])
                except Exception:pass
