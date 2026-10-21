#!/usr/bin/env python3
"""Owned synthetic PostgreSQL/storage/backend acceptance. Never uses Compose/.env.
Required: --backend-image, --postgres-image, --storage-image (already local).
Optional --cert/--key use existing localhost TLS material read-only, with no trust changes.
"""
import shutil, tempfile, stat as filemode
import re
import argparse, hashlib, http.client, http.cookiejar, http.server, json, os, pathlib, secrets, ssl, subprocess, threading, time, urllib.request, urllib.error, urllib.parse, uuid

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--backend-image',required=True);ap.add_argument('--rollback-image');ap.add_argument('--postgres-image',required=True);ap.add_argument('--storage-image',required=True);ap.add_argument('--output',required=True);ap.add_argument('--cert');ap.add_argument('--key');ap.add_argument('--ca');ap.add_argument('--hash-layout',choices=['bigint','bytea'],default='bigint');ap.add_argument('--browser-module');ap.add_argument('--hold-seconds',type=int,default=0);args=ap.parse_args()
    os.umask(0o077);root=pathlib.Path(args.output);root.mkdir(parents=True,exist_ok=False)
    repo=pathlib.Path(__file__).resolve().parents[2];name='sfd-auth-'+secrets.token_hex(6);label='sfd.auth-fixture';created=[];volumes=[];network=None;server=None;thread=None;native_dir=None
    result={'fixture':name,'source_revision':subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip(),'checks':[],'failures':[]};checks=result['checks']
    def check(value,name):
        if not value:raise AssertionError(name)
        checks.append(name)
    def command(argv,**kw):
        r=subprocess.run(argv,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=kw.pop('timeout',90),**kw)
        if r.returncode:
            if argv[0]=='go':(root/'private-go-failure.txt').write_bytes(r.stdout+r.stderr)
            if argv[0]=='node':
                for line in r.stderr.decode(errors='replace').splitlines():
                    if line.startswith('BROWSER DIAGNOSTIC: '):
                        diagnostic=json.loads(line.split(': ',1)[1]);result['browser_diagnostic']=diagnostic
            raise RuntimeError('fixture command failed: '+pathlib.Path(argv[0]).name+' '+argv[1])
        return r.stdout.decode().strip()
    def docker(*argv,**kw):return command(['docker',*argv],**kw)
    def owned(kind,ident):
        x=json.loads(docker(kind,'inspect',ident))[0];labels=x.get('Config',{}).get('Labels',{}) if kind=='container' else x.get('Labels',{});assert labels.get(label)==name;return x
    def snapshot():
        ids=docker('ps','-aq','--filter','label=com.docker.compose.project=securefiledrop').split()
        if not ids:return []
        values=json.loads(docker('inspect',*ids));return sorted([{'id':x['Id'],'image':x['Image'],'started':x['State']['StartedAt'],'mounts':sorted(x['Mounts'],key=lambda m:m['Destination'])} for x in values],key=lambda x:x['id'])
    baseline=snapshot()
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self,*args,**kwargs):raise urllib.error.URLError('fixture redirect denied')
    class Relay(http.server.BaseHTTPRequestHandler):
        backend=None
        def log_message(self,*a):pass
        def request(self):
            c=http.client.HTTPConnection('127.0.0.1',self.backend,timeout=20);body=self.rfile.read(int(self.headers.get('Content-Length','0')));headers={k:v for k,v in self.headers.items() if k.lower() not in ['connection','content-length','host']};headers['Host']=self.headers['Host'];headers['X-Forwarded-Proto']='https';c.request(self.command,self.path,body,headers);r=c.getresponse();body=r.read();self.send_response(r.status)
            for k,v in r.getheaders():
                if k.lower() not in ['connection','content-length','transfer-encoding','server','date']:self.send_header(k,v)
            self.send_header('Content-Length',str(len(body)));self.end_headers()
            if self.command!='HEAD':self.wfile.write(body)
            c.close()
        do_GET=request;do_POST=request;do_DELETE=request;do_PATCH=request;do_HEAD=request
    try:
        endpoint=docker('context','inspect','--format','{{.Endpoints.docker.Host}}');check(endpoint.startswith(('unix://','npipe://')) and not os.environ.get('DOCKER_HOST'),'local Docker target')
        cert=pathlib.Path(args.cert) if args.cert else root/'fixture.crt';key=pathlib.Path(args.key) if args.key else root/'fixture.key'
        if not args.cert:command(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','1','-subj','/CN=localhost','-addext','subjectAltName=DNS:localhost','-keyout',str(key),'-out',str(cert)])
        server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Relay);server.handle_error=lambda request,address:None;ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);ctx.load_cert_chain(cert,key);server.socket=ctx.wrap_socket(server.socket,server_side=True);origin='https://localhost:'+str(server.server_port)
        network=docker('network','create','--label',label+'='+name,name);owned('network',network)
        for suffix in ['pg','storage']:
            v=docker('volume','create','--label',label+'='+name,name+'-'+suffix);owned('volume',v);volumes.append(v)
        dbname='sfd_auth_'+secrets.token_hex(5);dbpass=secrets.token_hex(24);access='fixture'+secrets.token_hex(6);storagepass=secrets.token_hex(24);password='Fixture7'+secrets.token_hex(24)
        # Generate the bcrypt hash with the same supported Go dependency as the backend.
        helper=root/'hash.go';helper.write_text('package main\nimport("fmt";"os";"golang.org/x/crypto/bcrypt")\nfunc main(){ b,e:=bcrypt.GenerateFromPassword([]byte(os.Getenv("FIXTURE_PASSWORD")),12);if e!=nil{panic("fixture hash")};fmt.Print(string(b))}\n')
        env=dict(os.environ,GOTOOLCHAIN='go1.26.10',FIXTURE_PASSWORD=password);adminhash=command(['go','run',str(helper)],cwd=repo,env=env)
        values={'pg':{'POSTGRES_USER':'fixture','POSTGRES_DB':dbname,'POSTGRES_PASSWORD':dbpass},'storage':{'MINIO_ROOT_USER':access,'MINIO_ROOT_PASSWORD':storagepass},'backend':{'SFD_ADDR':':8080','DATABASE_URL':'postgres://fixture:'+dbpass+'@'+name+'-pg:5432/'+dbname+'?sslmode=disable','SFD_ADMIN_USER':'admin','SFD_ADMIN_PASS':adminhash,'SFD_SESSION_SECRET':secrets.token_hex(32),'SFD_DOWNLOAD_SECRET':secrets.token_hex(32),'SFD_S3_ENDPOINT':'http://'+name+'-storage:9000','SFD_S3_ACCESS_KEY':access,'SFD_S3_SECRET_KEY':storagepass,'SFD_BUCKET':name,'SFD_HASH_TOOL':'/app/sfd-hash','SFD_PUBLIC_BASE_URL':origin,'SFD_BASE_URL':origin,'SFD_MAX_UPLOAD_BYTES':'1024','SFD_MAX_REQUEST_BYTES':str(1024+(1<<20)),'SFD_CLEANUP_ENABLED':'false','SFD_BACKUP_ENABLED':'false','PGHOST':name+'-pg','PGPORT':'5432','PGUSER':'fixture','PGPASSWORD':dbpass,'PGDATABASE':dbname,'PGSSLMODE':'disable'}}
        for service,v in values.items():(root/(service+'.env')).write_text(''.join(k+'='+x+'\n' for k,x in v.items()))
        pg=docker('run','-d','--pull=never','--label',label+'='+name,'--name',name+'-pg','--network',network,'--env-file',str(root/'pg.env'),'-p','127.0.0.1::5432','-v',volumes[0]+':/var/lib/postgresql/data',args.postgres_image);created.append(pg);pginfo=owned('container',pg)
        for _ in range(60):
            r=subprocess.run(['docker','exec',pg,'pg_isready','-h','127.0.0.1','-U','fixture','-d',dbname],capture_output=True)
            if r.returncode==0:break
            time.sleep(.5)
        assert r.returncode==0, "PostgreSQL readiness"
        def sql(text):return docker('exec','-i',pg,'psql','-X','-q','-A','-t','-v','ON_ERROR_STOP=1','-U','fixture','-d',dbname,input=text.encode())
        # Reproduce the existing version-11 schema, including its manual role column.
        for path in sorted((repo/'internal/db/migrations').glob('*.up.sql')):
            if int(path.name.split('_')[0])<=11:sql(path.read_text())
        sql("ALTER TABLE users ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT false; CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL); INSERT INTO schema_migrations VALUES(11,false); CREATE TABLE sfd_test_owner(owner text NOT NULL); INSERT INTO sfd_test_owner VALUES('"+name+"');")
        if args.hash_layout=='bytea':sql('ALTER TABLE files DROP CONSTRAINT files_sha256_bytes_check; ALTER TABLE files ALTER COLUMN sha256_bytes TYPE bytea USING NULL::bytea;')
        result['hash_metadata_layout']=args.hash_layout
        sentinel=str(uuid.uuid4());sql("INSERT INTO users(id,email,username,password_hash,is_admin,email_verified) VALUES('"+sentinel+"','sentinel@example.invalid','sentinel','not-a-credential',true,true);")
        sentinel_file=str(uuid.uuid4());sentinel_digest='ab'*32
        sentinel_value="decode('"+sentinel_digest+"','hex')" if args.hash_layout=='bytea' else '1'
        sql("INSERT INTO files(id,object_key,orig_name,content_type,size_bytes,status,user_id,sha256_hex,sha256_bytes) VALUES('"+sentinel_file+"','sentinel/source','preserved.txt','text/plain',1,'hashed','"+sentinel+"','"+sentinel_digest+"',"+sentinel_value+");")
        def fingerprint():return hashlib.sha256(sql("SELECT row_to_json(u)::text FROM users u ORDER BY id; SELECT row_to_json(f)::text FROM files f ORDER BY id;").encode()).hexdigest()
        before=fingerprint()
        storage=docker('run','-d','--pull=never','--label',label+'='+name,'--name',name+'-storage','--network',network,'-p','127.0.0.1::9000','--env-file',str(root/'storage.env'),'-v',volumes[1]+':/data',args.storage_image,'server','/data');created.append(storage);owned('container',storage)
        for _ in range(60):
            r=subprocess.run(['docker','exec',storage,'mc','alias','set','fixture','http://127.0.0.1:9000',access,storagepass],capture_output=True)
            if r.returncode==0:break
            time.sleep(.5)
        assert r.returncode==0, 'storage alias readiness';docker('exec',storage,'mc','mb','fixture/'+name)
        def start_backend(image=None):
            b=docker('run','-d','--pull=never','--label',label+'='+name,'--network',network,'--stop-timeout','25','--env-file',str(root/'backend.env'),'-p','127.0.0.1::8080',image or args.backend_image);created.append(b);i=owned('container',b);ports=i['NetworkSettings'].get('Ports',{}).get('8080/tcp')
            if not ports:
                if image is None:raise RuntimeError('candidate exited before loopback binding')
                Relay.backend=0;return b
            assert len(ports)==1 and ports[0]['HostIp']=='127.0.0.1';Relay.backend=int(ports[0]['HostPort']);return b
        backend=start_backend();thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
        trust=ssl.create_default_context(cafile=str(pathlib.Path(args.ca) if args.ca else cert));jar=http.cookiejar.CookieJar();client=urllib.request.build_opener(NoRedirect(),urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=trust),urllib.request.HTTPCookieProcessor(jar))
        def request(path,body=None,method=None,headers=None,who=client):
            h={'Origin':origin};h.update(headers or {})
            if isinstance(body,dict):body=json.dumps(body).encode();h['Content-Type']='application/json'
            req=urllib.request.Request(origin+path,body,h,method=method)
            try:
                with who.open(req,timeout=35) as r:return r.status,r.read()
            except urllib.error.HTTPError as e:return e.code,e.read()
        def ready():
            for _ in range(60):
                try:
                    if request('/ready')[0]==200:return
                except (urllib.error.URLError,ConnectionError):pass
                time.sleep(.5)
            raise AssertionError('corrected backend readiness')
        ready()
        owned('container',backend)
        dump=docker('exec',backend,'pg_dump','--format=plain','--no-owner','--no-acl','--no-password')
        check('CREATE TABLE public.users' in dump and 'CREATE TABLE public.auth_sessions' in dump,'actual pg_dump accepts separated connection environment without credential arguments')
        del dump
        check(fingerprint()==before,'migration 11 to 12 preserves synthetic user/file records and role');check(sql('SELECT version FROM schema_migrations;')=='12','migration reaches version 12')
        # Retained canonical bounds are checked on the actual selected image, without credentials.
        check(request('/login',b'{}{}',method='POST',headers={'Content-Type':'application/json'})[0]==400,'real login rejects trailing JSON')
        check(request('/login',b' '*(16385),method='POST',headers={'Content-Type':'application/json'})[0]==413,'real login bounds total JSON including whitespace')
        check(request('/register',b'{}{}',method='POST',headers={'Content-Type':'application/json'})[0]==400,'real registration rejects trailing JSON')
        # Exercise actual HTTP login, identity, privileged reads, logout and replay.
        check(request('/login',{'username':'admin','password':password})[0]==200,'real environment admin login')
        status,body=request('/me');me=json.loads(body);check(status==200 and me['username']=='admin' and me['is_admin'] is True,'real environment admin identity')
        check(request('/admin/files')[0]==200,'environment admin privileged read')
        check(request('/files',{'orig_name':'admin.txt','content_type':'text/plain','size_bytes':1})[0]==403,'environment identity cannot acquire database ownership')
        check(request('/files')[0]==403,'environment identity cannot acquire owned-file listing')
        oldcookie=next(iter(jar)).value;check(request('/logout',{},method='POST')[0]==200,'real logout')
        anonymous=urllib.request.build_opener(NoRedirect(),urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=trust));check(request('/me',headers={'Cookie':'sfd_session='+oldcookie},who=anonymous)[0]==401,'real logout rejects replay')
        owned('container',backend);docker('rm','-f',backend);created.remove(backend);backend=start_backend();ready();check(request('/me',headers={'Cookie':'sfd_session='+oldcookie},who=anonymous)[0]==401,'logout revocation survives backend recreation')
        # Run the exact handler/security suite against this uniquely guarded database.
        pgports=pginfo['NetworkSettings']['Ports']['5432/tcp'];assert pgports[0]['HostIp']=='127.0.0.1'
        # DrvFS build outputs can report mode0777. Preserve executable policy:
        # stage exact compiled bytes in a new private Linux directory, never
        # relax the application's group/other-write rejection.
        result['stage']='host-native-permission-staging'
        native_dir=pathlib.Path(tempfile.mkdtemp(prefix='sfd-lifecycle-native-',dir='/tmp'));native_dir.chmod(0o700)
        (native_dir/'owner').write_text(name)
        native_bytes=(repo/'native/sfd-hash').read_bytes();native_tool=native_dir/'sfd-hash'
        with native_tool.open('xb') as output:output.write(native_bytes)
        native_tool.chmod(0o500)
        check(hashlib.sha256(native_tool.read_bytes()).digest()==hashlib.sha256(native_bytes).digest() and filemode.S_IMODE(native_tool.stat().st_mode)==0o500,'exact host-native bytes staged with safe executable permissions')
        result['host_native_tool_sha256']=hashlib.sha256(native_bytes).hexdigest()
        testenv=dict(os.environ,GOTOOLCHAIN='go1.26.10',CGO_ENABLED='1',SFD_AUTH_TEST_DSN='postgres://fixture:'+dbpass+'@127.0.0.1:'+pgports[0]['HostPort']+'/'+dbname+'?sslmode=disable',SFD_AUTH_TEST_OWNER=name,SFD_LIFECYCLE_DATABASE=dbname,SFD_LIFECYCLE_BUCKET=name,SFD_LIFECYCLE_STORAGE=docker('port',storage,'9000/tcp'),SFD_LIFECYCLE_ACCESS=access,SFD_LIFECYCLE_SECRET=storagepass,SFD_HASH_TOOL=str(native_tool))
        result['stage']='real-go-postgresql-native-commit-faults'
        output=command(['go','test','-race','./internal/...','./cmd/...','-count=1','-v'],cwd=repo,env=testenv,timeout=180);(root/'go-tests.txt').write_text(output+'\n');result['go_security']={'passed_test_events':len(re.findall(r'(?m)^\s*--- PASS: ',output)),'skipped_test_events':len(re.findall(r'(?m)^\s*--- SKIP: ',output)),'failed_test_events':len(re.findall(r'(?m)^\s*--- FAIL: ',output)),'real_lineage_subtests':len(re.findall(r'(?m)^\s*--- PASS: TestLegacyLineage/',output)),'real_lifecycle_ordering_cases':len(re.findall(r'(?m)^\s*--- PASS: TestLifecycleOrdering/',output)),'real_commit_fault_modes':len(re.findall(r'(?m)^\s*--- PASS: TestLegacyCommitFaults/',output))}
        check('--- PASS: TestAuthenticatedIdentities' in output and '--- SKIP:' not in output and result['go_security']['real_lineage_subtests']==14 and result['go_security']['real_commit_fault_modes']==4 and result['go_security']['real_lifecycle_ordering_cases']==5,'real PostgreSQL Go identity/lineage/COMMIT regressions and race checks')
        # Separate ordinary browser/API account created only in the fixture.
        uploader_password='Synthetic9-'+secrets.token_hex(24)
        check(request('/register',{'email':'upload@example.invalid','username':'uploader','password':uploader_password})[0]==201,'real registration')
        sql("UPDATE users SET email_verified=true WHERE username='uploader';")
        check(request('/login',{'username':'uploader','password':uploader_password})[0]==200,'real registered-user login')
        status,body=request('/me');check(status==200 and json.loads(body)['is_admin'] is False,'registered identity readback')
        check(request('/admin/files')[0]==403,'registered account denied admin permission')
        check(request('/metrics')[0]==403,'registered account denied aggregate administrator metrics')
        check(request('/files',{'orig_name':'bad\nname.txt','content_type':'text/plain','size_bytes':0})[0]==400,'real metadata rejects control characters')
        check(request('/files',{'orig_name':'a'*256,'content_type':'text/plain','size_bytes':0})[0]==400,'real metadata rejects oversized filenames without truncation')
        data=b'Secure File Drop isolated native hash fixture\n';status,body=request('/files',{'orig_name':'fixture.txt','content_type':'text/plain','size_bytes':len(data)});check(status==201,'owned metadata creation');fid=json.loads(body)['id']
        boundary='sfd'+secrets.token_hex(12);multipart=('--'+boundary+'\r\nContent-Disposition: form-data; name="file"; filename="fixture.txt"\r\nContent-Type: text/plain\r\n\r\n').encode()+data+('\r\n--'+boundary+'--\r\n').encode()
        status,body=request('/upload?id='+fid,multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary});check(status==200,'real native upload and hash');digest=sql("SELECT sha256_hex FROM files WHERE id='"+fid+"';");check(digest==hashlib.sha256(data).hexdigest(),'native SHA-256 matches uploaded bytes')
        if args.hash_layout=='bytea':metadata=sql("SELECT encode(sha256_bytes,'hex') FROM files WHERE id='"+fid+"';");check(metadata==hashlib.sha256(data).hexdigest(),'legacy raw digest metadata preserved')
        else:metadata=sql("SELECT sha256_bytes FROM files WHERE id='"+fid+"';");check(metadata==str(len(data)),'numeric byte-count metadata correct')
        status,body=request('/links',{'id':fid,'ttl_seconds':60});check(status==200,'owned signed link');link=json.loads(body)['url'];target=urllib.parse.urlsplit(link)
        check(target.scheme=='https' and target.hostname=='localhost' and target.port==server.server_port and target.username is None and target.password is None and target.path=='/download' and not target.fragment,'signed download target confined to owned HTTPS origin')
        req=urllib.request.Request(link)
        with anonymous.open(req,timeout=10) as r:check(r.status==200 and r.read()==data,'signed download bytes match')
        # Preserve existing protected-row interpretation; this is a synthetic
        # historical fixture, not a newly implemented password-creation feature.
        sql("UPDATE files SET link_password='owned-historical-password' WHERE id='"+fid+"';")
        download_path=target.path+'?'+target.query
        check(request(download_path,who=anonymous)[0]==401,'existing protected record denies absent password')
        check(request(download_path+'&password=wrong-owned-password',who=anonymous)[0]==401,'existing protected record denies incorrect password')
        code,body=request(download_path+'&password=owned-historical-password',who=anonymous)
        check(code==200 and body==data,'existing protected record retains exact legacy password interpretation')
        sql("UPDATE files SET link_password=NULL,auto_delete=TRUE,expires_at=NOW()-INTERVAL '1 minute' WHERE id='"+fid+"';")
        check(request(download_path,who=anonymous)[0]==410,'explicitly expired file denies still-valid bearer link')
        sql("UPDATE files SET auto_delete=FALSE,expires_at=NULL WHERE id='"+fid+"';")
        check(request('/files')[0]==200 and json.loads(request('/files')[1])==json.loads(request('/user/files')[1]) and fid.encode() in request('/files')[1],'companion GET files reuses owned listing and native metadata')
        check(request('/files',who=anonymous)[0]==401,'anonymous companion file listing rejected')
        # Each phase uses a newly recreated owned backend, retaining the real
        # persisted sessions. Production rate limits are unchanged; this avoids
        # combining unrelated test cases into one limiter window.
        def restart_phase(label_name):
            nonlocal backend
            owned('container',backend);docker('rm','-f',backend);created.remove(backend)
            backend=start_backend();ready()
            check(request('/me')[0]==200,label_name+' preserves the existing verified session')
        result['stage']='real-https-lifecycle-preservation'
        restart_phase('lifecycle phase restart')
        check(request('/upload/resumable',{},method='POST')[0]==501,'unaccepted resumable path fails closed for registered users')
        check(request('/upload/resumable',{},method='POST',who=anonymous)[0]==401,'unsupported path still requires authentication')
        jar.clear();check(request('/login',{'username':'admin','password':password})[0]==200,'lifecycle administrator login')
        admin_status,_=request('/admin/files/'+fid,method='DELETE')
        admin_row=sql("SELECT EXISTS(SELECT 1 FROM files WHERE id='"+fid+"');")=='t'
        missing=subprocess.run(['docker','exec',storage,'mc','stat','fixture/'+name+'/uploads/'+fid],capture_output=True).returncode!=0
        check(admin_status==204 and not admin_row and missing,'administrator deletes exact stored object and metadata')
        check(request('/upload/resumable',{},method='POST')[0]==403,'environment identity cannot acquire unsupported UUID ownership')
        jar.clear();check(request('/login',{'username':'uploader','password':uploader_password})[0]==200,'lifecycle owner login')
        def create_file(filename,size):
            status,body=request('/files',{'orig_name':filename,'content_type':'text/plain','size_bytes':size})
            check(status==201,'owned lifecycle metadata '+filename);return json.loads(body)['id']
        def upload_bytes(file_id,value,complete=True):
            boundary='sfd'+secrets.token_hex(12)
            body=('--'+boundary+'\r\nContent-Disposition: form-data; name="file"; filename="fixture.txt"\r\nContent-Type: text/plain\r\n\r\n').encode()+value
            if complete:body+=('\r\n--'+boundary+'--\r\n').encode()
            return request('/upload?id='+file_id,body,headers={'Content-Type':'multipart/form-data; boundary='+boundary})
        fid=create_file('storage-failure.txt',len(data));check(upload_bytes(fid,data)[0]==200,'real file exists before storage-failure deletion')
        before_delete=hashlib.sha256(sql("SELECT row_to_json(f)::text FROM files f WHERE id='"+fid+"';").encode()).hexdigest()
        owned('container',storage);docker('stop',storage)
        failure_status,_=request('/user/files/'+fid,method='DELETE')
        after_delete=hashlib.sha256(sql("SELECT row_to_json(f)::text FROM files f WHERE id='"+fid+"';").encode()).hexdigest()
        owned('container',storage);docker('start',storage)
        for _ in range(60):
            stat=subprocess.run(['docker','exec',storage,'mc','stat','fixture/'+name+'/uploads/'+fid],capture_output=True)
            if stat.returncode==0:break
            time.sleep(.5)
        check(failure_status==502 and before_delete==after_delete and stat.returncode==0,'storage outage retains exact file metadata and actual object')
        result['deletion_preservation']={'admin_status':admin_status,'admin_object_removed':missing,'storage_failure_status':failure_status,'row_hash_unchanged':before_delete==after_delete,'object_retained':stat.returncode==0}
        # Observe a real owned row lock during an unfinished multipart body,
        # then send SIGTERM only to this fixture backend. Its process must leave
        # time for storage/metadata compensation before a clean exit.
        shutdown_id=create_file('active-shutdown.txt',3)
        active_info=owned('container',backend);active_port=active_info['NetworkSettings']['Ports']['8080/tcp'][0]
        assert active_port['HostIp']=='127.0.0.1'
        stream=http.client.HTTPConnection('127.0.0.1',int(active_port['HostPort']),timeout=30)
        shutdown_boundary='sfd-shutdown-'+secrets.token_hex(8)
        fragment=('--'+shutdown_boundary+'\r\nContent-Disposition: form-data; name="file"; filename="active-shutdown.txt"\r\nContent-Type: text/plain\r\n\r\nabc').encode()
        try:
            stream.putrequest('POST','/upload?id='+shutdown_id)
            stream.putheader('Origin',origin);stream.putheader('Cookie','; '.join(c.name+'='+c.value for c in jar))
            stream.putheader('Content-Type','multipart/form-data; boundary='+shutdown_boundary);stream.putheader('Content-Length',str(len(fragment)+100));stream.endheaders();stream.send(fragment)
            locked=False
            for _ in range(30):
                owned('container',name+'-pg')
                lock_probe=subprocess.run(['docker','exec',name+'-pg','psql','-X','-q','-A','-t','-v','ON_ERROR_STOP=1','-v','VERBOSITY=sqlstate','-U','fixture','-d',dbname,'-c',"BEGIN; SELECT id FROM files WHERE id='"+shutdown_id+"' FOR UPDATE NOWAIT;ROLLBACK;"],capture_output=True)
                if lock_probe.returncode!=0:
                    assert b'55P03' in lock_probe.stderr,'unexpected owned lock diagnostic'
                    locked=True;break
                time.sleep(.1)
            check(locked,'active multipart owns real PostgreSQL row lock before SIGTERM')
            started=time.monotonic();owned('container',backend);docker('stop',backend);elapsed=time.monotonic()-started
        finally:stream.close()
        final_info=owned('container',backend)
        status_after=sql("SELECT status FROM files WHERE id='"+shutdown_id+"';")
        object_missing=subprocess.run(['docker','exec',storage,'mc','stat','fixture/'+name+'/uploads/'+shutdown_id],capture_output=True).returncode!=0
        check(final_info['Config']['StopTimeout']==25 and final_info['State']['ExitCode']==0 and not final_info['State']['Running'] and elapsed<20 and status_after=='failed' and object_missing,'real SIGTERM finishes bounded failed-upload compensation and exits cleanly')
        result['active_shutdown']={'exit_code':final_info['State']['ExitCode'],'elapsed_seconds':round(elapsed,3),'row_lock_observed':locked,'status':status_after,'actual_object_absent':object_missing,'configured_docker_stop_timeout_seconds':final_info['Config']['StopTimeout'],'docker_kill_budget_seconds':25,'application_budget_seconds':20}
        restart_phase('post-SIGTERM phase restart')
        mismatch=create_file('mismatch.txt',1);check(upload_bytes(mismatch,b'abc')[0]==400,'actual bytes must match reserved metadata')
        check(sql("SELECT status FROM files WHERE id='"+mismatch+"';")=='failed','mismatched upload retains failed metadata')
        truncated=create_file('truncated.txt',len(data));check(upload_bytes(truncated,data,False)[0]==400,'truncated multipart framing rejected')
        check(sql("SELECT status FROM files WHERE id='"+truncated+"';")=='failed','truncated upload cannot become hashed')
        boundary_id=create_file('boundary.txt',1024);check(upload_bytes(boundary_id,b'x'*1024)[0]==200,'actual backend exact file boundary accepts multipart overhead')
        check(request('/files',{'orig_name':'over.txt','content_type':'text/plain','size_bytes':1025})[0]==413,'oversized metadata rejected before storage')
        check(request('/files',{'orig_name':'unaccepted-password.txt','content_type':'text/plain','size_bytes':1,'link_password':'synthetic-only'})[0]==400,'unsupported password creation cannot silently issue an unprotected file')
        check(request('/files',{'orig_name':'origin.txt','content_type':'text/plain','size_bytes':1},headers={'Origin':'https://not-this-fixture.invalid'})[0]==403,'real foreign-origin mutation rejected')
        quota_password='Quota7-'+secrets.token_hex(24);quota_jar=http.cookiejar.CookieJar();quota_client=urllib.request.build_opener(NoRedirect(),urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=trust),urllib.request.HTTPCookieProcessor(quota_jar))
        check(request('/register',{'email':'quota@example.invalid','username':'quota_probe','password':quota_password},who=quota_client)[0]==201,'synthetic quota account registration')
        sql("UPDATE users SET email_verified=true,storage_quota_bytes=16 WHERE username='quota_probe';")
        check(request('/login',{'username':'quota_probe','password':quota_password},who=quota_client)[0]==200,'synthetic quota account login')
        status,body=request('/files',{'orig_name':'reserved-a.txt','content_type':'text/plain','size_bytes':10},who=quota_client);check(status==201,'pending metadata reserves existing quota');reserved=json.loads(body)['id']
        check(request('/files',{'orig_name':'reserved-b.txt','content_type':'text/plain','size_bytes':10},who=quota_client)[0]==403,'simultaneous pending reservations cannot overrun quota')
        check(request('/user/files/'+reserved,method='DELETE',who=quota_client)[0]==204,'owner can release an unused pending reservation')
        check(request('/files',{'orig_name':'reserved-c.txt','content_type':'text/plain','size_bytes':10},who=quota_client)[0]==201,'released quota becomes available')
        restart_phase('post-lifecycle phase restart')
        # Distinct account must not read, upload, link or delete the first account file.
        second_password='Synthetic9-'+secrets.token_hex(24)
        check(request('/register',{'email':'second@example.invalid','username':'second','password':second_password})[0]==201,'second isolated account');sql("UPDATE users SET email_verified=true WHERE username='second';")
        check(request('/login',{'username':'second','password':second_password})[0]==200,'second account login')
        check(request('/links',{'id':fid})[0]==404,'cross-account link denied');check(request('/upload?id='+fid,multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})[0]==404,'cross-account upload denied');check(request('/user/files/'+fid,method='DELETE')[0]==404,'cross-account deletion denied')
        check(request('/user/files')[0]==200 and fid.encode() not in request('/user/files')[1],'file list isolates accounts')
        check(request('/files')[0]==200 and fid.encode() not in request('/files')[1],'companion file listing isolates different accounts')
        (root/'windows-fixture.json').write_text(json.dumps({'Origin':origin,'Username':'admin','Password':password,'FixtureOwner':name,'Uploader':'uploader','UploaderPassword':uploader_password,'Other':'second','OtherPassword':second_password}))

        if args.browser_module:
            # This trust setup is restricted to a disposable GitHub Actions runner.
            # It never runs on the Owner's Windows/WSL environment or changes its CA.
            check(os.environ.get('GITHUB_ACTIONS')=='true' and not args.cert,'ephemeral CI fixture trust only')
            nss=pathlib.Path.home()/'.pki/nssdb'
            nss.mkdir(parents=True,exist_ok=True)
            if not (nss/'cert9.db').exists():command(['certutil','-N','-d','sql:'+str(nss),'--empty-password'])
            command(['certutil','-A','-d','sql:'+str(nss),'-n',name,'-t','C,,','-i',str(cert)])
            browserenv=dict(os.environ,SFD_AUTH_BROWSER_MODULE=str(pathlib.Path(args.browser_module).resolve()),NODE_EXTRA_CA_CERTS=str(pathlib.Path(args.ca) if args.ca else cert))
            browserout=command(['node',str(repo/'tests/auth/browser.cjs'),origin,str(root/'windows-fixture.json')],env=browserenv,timeout=60)
            (root/'browser-tests.txt').write_text(browserout+'\n')
            result['browser']=json.loads(next(line.split(': ',1)[1] for line in browserout.splitlines() if line.startswith('LEGACY BROWSER CHECKS: ')))
            check('BROWSER ACCEPTANCE PASS:' in browserout,'real historical browser UI over normally validated HTTPS')
            command(['certutil','-D','-d','sql:'+str(nss),'-n',name])
        result['origin']=origin;result['backend_image']=args.backend_image;result['backend_image_id']=owned('container',backend)['Image'];result['certificate_sha256']=hashlib.sha256(ssl.PEM_cert_to_DER_cert(cert.read_text())).hexdigest();(root/'ready.json').write_text(json.dumps({'origin':origin,'ready':True}))
        deadline=time.monotonic()+args.hold_seconds
        while time.monotonic()<deadline and not (root/'continue').exists():
            control=root/'release-control'
            if control.exists():
                assert control.is_dir() and (control/'owner').read_text()==name,'native control owner mismatch'
                if (control/'interrupt-backend.request').exists() and not (control/'backend-stopped').exists():
                    owned('container',backend);docker('stop',backend)
                    assert not owned('container',backend)['State']['Running']
                    with (control/'backend-stopped').open('x') as ack:ack.write('stopped')
                if (control/'restore-backend.request').exists() and not (control/'backend-restored').exists():
                    owned('container',backend);docker('start',backend);info=owned('container',backend)
                    ports=info['NetworkSettings']['Ports']['8080/tcp'];assert len(ports)==1 and ports[0]['HostIp']=='127.0.0.1'
                    Relay.backend=int(ports[0]['HostPort']);ready()
                    with (control/'backend-restored').open('x') as ack:ack.write('ready')
            time.sleep(.05)
        # Run only after browser/Windows acceptance so this bounded negative check
        # cannot consume their normal login budget. No account/password is used.
        limited=False
        for _ in range(11):
            req=urllib.request.Request(origin+'/login',b'not-json',{'Origin':origin,'Content-Type':'application/json'},method='POST')
            try:
                with anonymous.open(req,timeout=10) as response:response.read()
            except urllib.error.HTTPError as error:
                error.read()
                if error.code==429 and error.headers.get('X-RateLimit-Limit-Type')=='authentication' and error.headers.get('Retry-After'):
                    limited=True;break
        check(limited,'real auth endpoint rate limiter rejects excess malformed requests')
        if args.rollback_image:
            # Only assignment-owned containers and synthetic data are touched.
            before_rollback=fingerprint()
            sessions_before=hashlib.sha256(sql('SELECT token_hash,expires_at FROM auth_sessions ORDER BY token_hash;').encode()).hexdigest()
            rollback_ready=False;rollback_security=[]
            owned('container',backend);docker('rm','-f',backend);created.remove(backend)
            backend=start_backend(args.rollback_image)
            try:
                for _ in range(30):
                    try:
                        if request('/ready',who=anonymous)[0]==200:rollback_ready=True;break
                    except (urllib.error.URLError,ConnectionError):pass
                    if not owned('container',backend)['State']['Running']:break
                    time.sleep(.5)
                result['rollback']={'image':args.rollback_image,'image_id':owned('container',backend)['Image'],'ready':rollback_ready,'schema_version':sql('SELECT version FROM schema_migrations;')}
                check(fingerprint()==before_rollback and hashlib.sha256(sql('SELECT token_hash,expires_at FROM auth_sessions ORDER BY token_hash;').encode()).hexdigest()==sessions_before and sql('SELECT version FROM schema_migrations;')=='12','prior image rollback preserves additive schema and all synthetic records/sessions')
                if rollback_ready:
                    jar.clear()
                    status,_=request('/login',{'username':'admin','password':password})
                    status_me,body=request('/me')
                    rollback_security.append((status==200 and status_me==200 and json.loads(body)['is_admin'] is True,'rollback environment administrator login and identity'))
                    rollback_cookie=next(iter(jar)).value
                    status,_=request('/logout',{},method='POST')
                    replay,_=request('/me',headers={'Cookie':'sfd_session='+rollback_cookie},who=anonymous)
                    rollback_security.append((status==200 and replay==401,'rollback logout revokes session replay'))
                    status,_=request('/login',{'username':'uploader','password':uploader_password})
                    status_me,body=request('/me')
                    denied,_=request('/admin/files')
                    rollback_security.append((status==200 and status_me==200 and json.loads(body)['is_admin'] is False and denied==403,'rollback registered identity and administrator isolation'))
                    statuses=[];locked_body=b''
                    for _ in range(6):
                        code,locked_body=request('/login',{'username':'admin','password':'incorrect-fixture-only'})
                        statuses.append(code)
                    result['rollback']['lockout_statuses']=statuses
                    result['rollback']['account_lockout_enforced']=statuses==[401,401,401,401,401,429] and locked_body.startswith(b'Account locked until ')
                    # A different account's correct login must remain usable.
                    isolated,_=request('/login',{'username':'uploader','password':uploader_password})
                    rollback_security.append((isolated==200,'rollback lockout isolates accounts'))
                    rollback_security.append((result['rollback']['account_lockout_enforced'],'rollback account lockout is enforced before the IP request ceiling'))
            finally:
                owned('container',backend);docker('rm','-f',backend);created.remove(backend);backend=start_backend();ready()
            status,_=request('/login',{'username':'admin','password':password});status_me,body=request('/me')
            check(status==200 and status_me==200 and json.loads(body)['is_admin'] is True,'corrected candidate restored after rollback image test')
            check(rollback_ready,'selected rollback image starts against additive schema 12')
            for accepted,description in rollback_security:check(accepted,description)


    except Exception as e:
        import traceback
        result['failure_location']=[{'file':pathlib.Path(f.filename).name,'line':f.lineno,'function':f.name} for f in traceback.extract_tb(e.__traceback__)]

        for ident in created:
            logs=subprocess.run(['docker','logs','--tail','30',ident],capture_output=True);(root/('private-diagnostic-'+ident[:12]+'.txt')).write_bytes(logs.stdout+logs.stderr)
        result['failures'].append(str(e) if isinstance(e,(AssertionError,RuntimeError)) else type(e).__name__)
    finally:
        if thread is not None:server.shutdown()
        if server is not None:server.server_close()
        for ident in reversed(created):owned('container',ident);docker('rm','-f',ident)
        if network is not None:owned('network',network);docker('network','rm',network)
        for v in reversed(volumes):owned('volume',v);docker('volume','rm',v)
        if native_dir is not None:
            assert native_dir.parent==pathlib.Path('/tmp') and not native_dir.is_symlink() and (native_dir/'owner').read_text()==name
            shutil.rmtree(native_dir)
        result['existing_services_unchanged']=snapshot()==baseline;result['owned_cleanup_complete']=not docker('ps','-aq','--filter','label='+label+'='+name) and not docker('volume','ls','-q','--filter','label='+label+'='+name) and not docker('network','ls','-q','--filter','label='+label+'='+name)
        (root/'result.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result,indent=2))
    return 0 if not result['failures'] and result['existing_services_unchanged'] and result['owned_cleanup_complete'] else 1
if __name__=='__main__':raise SystemExit(main())