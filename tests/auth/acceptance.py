#!/usr/bin/env python3
"""Owned synthetic PostgreSQL/storage/backend acceptance. Never uses Compose/.env.
Required: --backend-image, --postgres-image, --storage-image (already local).
Optional --cert/--key use existing localhost TLS material read-only, with no trust changes.
"""
import argparse, hashlib, http.client, http.cookiejar, http.server, json, os, pathlib, secrets, ssl, subprocess, threading, time, urllib.request, urllib.error, uuid

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--backend-image',required=True);ap.add_argument('--postgres-image',required=True);ap.add_argument('--storage-image',required=True);ap.add_argument('--output',required=True);ap.add_argument('--cert');ap.add_argument('--key');ap.add_argument('--ca');ap.add_argument('--browser-module');ap.add_argument('--hold-seconds',type=int,default=0);args=ap.parse_args()
    os.umask(0o077);root=pathlib.Path(args.output);root.mkdir(parents=True,exist_ok=False)
    repo=pathlib.Path(__file__).resolve().parents[2];name='sfd-auth-'+secrets.token_hex(6);label='sfd.auth-fixture';created=[];volumes=[];network=None;server=None;thread=None
    result={'fixture':name,'checks':[],'failures':[]};checks=result['checks']
    def check(value,name):
        if not value:raise AssertionError(name)
        checks.append(name)
    def command(argv,**kw):
        r=subprocess.run(argv,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=kw.pop('timeout',90),**kw)
        if r.returncode:raise RuntimeError('fixture command failed: '+pathlib.Path(argv[0]).name+' '+argv[1])
        return r.stdout.decode().strip()
    def docker(*argv,**kw):return command(['docker',*argv],**kw)
    def owned(kind,ident):
        x=json.loads(docker(kind,'inspect',ident))[0];labels=x.get('Config',{}).get('Labels',{}) if kind=='container' else x.get('Labels',{});assert labels.get(label)==name;return x
    def snapshot():
        ids=docker('ps','-aq','--filter','label=com.docker.compose.project=securefiledrop').split()
        if not ids:return []
        values=json.loads(docker('inspect',*ids));return sorted([{'id':x['Id'],'image':x['Image'],'started':x['State']['StartedAt'],'mounts':sorted(x['Mounts'],key=lambda m:m['Destination'])} for x in values],key=lambda x:x['id'])
    baseline=snapshot()
    class Relay(http.server.BaseHTTPRequestHandler):
        backend=None
        browser_html=None
        def log_message(self,*a):pass
        def request(self):
            if self.path=='/__sfd-owned-browser-test' and self.browser_html:
                body=self.browser_html.encode();self.send_response(200);self.send_header('Content-Type','text/html');self.send_header('Cache-Control','no-store');self.send_header('Content-Length',str(len(body)));self.end_headers();self.wfile.write(body);return
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
        env=dict(os.environ,GOTOOLCHAIN='go1.26.9',FIXTURE_PASSWORD=password);adminhash=command(['go','run',str(helper)],cwd=repo,env=env)
        values={'pg':{'POSTGRES_USER':'fixture','POSTGRES_DB':dbname,'POSTGRES_PASSWORD':dbpass},'storage':{'MINIO_ROOT_USER':access,'MINIO_ROOT_PASSWORD':storagepass},'backend':{'SFD_ADDR':':8080','DATABASE_URL':'postgres://fixture:'+dbpass+'@'+name+'-pg:5432/'+dbname+'?sslmode=disable','SFD_ADMIN_USER':'admin','SFD_ADMIN_PASS':adminhash,'SFD_SESSION_SECRET':secrets.token_hex(32),'SFD_DOWNLOAD_SECRET':secrets.token_hex(32),'SFD_S3_ENDPOINT':'http://'+name+'-storage:9000','SFD_S3_ACCESS_KEY':access,'SFD_S3_SECRET_KEY':storagepass,'SFD_BUCKET':name,'SFD_HASH_TOOL':'/app/sfd-hash','SFD_PUBLIC_BASE_URL':origin,'SFD_BASE_URL':origin,'SFD_CLEANUP_ENABLED':'false','SFD_BACKUP_ENABLED':'false'}}
        for service,v in values.items():(root/(service+'.env')).write_text(''.join(k+'='+x+'\n' for k,x in v.items()))
        pg=docker('run','-d','--pull=never','--label',label+'='+name,'--name',name+'-pg','--network',network,'--env-file',str(root/'pg.env'),'-p','127.0.0.1::5432','-v',volumes[0]+':/var/lib/postgresql/data',args.postgres_image);created.append(pg);pginfo=owned('container',pg)
        for _ in range(60):
            r=subprocess.run(['docker','exec',pg,'pg_isready','-U','fixture','-d',dbname],capture_output=True)
            if r.returncode==0:break
            time.sleep(.5)
        assert r.returncode==0, "PostgreSQL readiness"
        def sql(text):return docker('exec','-i',pg,'psql','-X','-q','-A','-t','-v','ON_ERROR_STOP=1','-U','fixture','-d',dbname,input=text.encode())
        # Reproduce the existing version-11 schema, including its manual role column.
        for path in sorted((repo/'internal/db/migrations').glob('*.up.sql')):
            if int(path.name.split('_')[0])<=11:sql(path.read_text())
        sql("ALTER TABLE users ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT false; CREATE TABLE schema_migrations(version bigint PRIMARY KEY,dirty boolean NOT NULL); INSERT INTO schema_migrations VALUES(11,false); CREATE TABLE sfd_test_owner(owner text NOT NULL); INSERT INTO sfd_test_owner VALUES('"+name+"');")
        sentinel=str(uuid.uuid4());sql("INSERT INTO users(id,email,username,password_hash,is_admin,email_verified) VALUES('"+sentinel+"','sentinel@example.invalid','sentinel','not-a-credential',true,true);")
        def fingerprint():return hashlib.sha256(sql("SELECT row_to_json(u)::text FROM users u ORDER BY id; SELECT row_to_json(f)::text FROM files f ORDER BY id;").encode()).hexdigest()
        before=fingerprint()
        storage=docker('run','-d','--pull=never','--label',label+'='+name,'--name',name+'-storage','--network',network,'--env-file',str(root/'storage.env'),'-v',volumes[1]+':/data',args.storage_image,'server','/data');created.append(storage);owned('container',storage)
        for _ in range(60):
            r=subprocess.run(['docker','exec',storage,'mc','alias','set','fixture','http://127.0.0.1:9000',access,storagepass],capture_output=True)
            if r.returncode==0:break
            time.sleep(.5)
        assert r.returncode==0, 'storage alias readiness';docker('exec',storage,'mc','mb','fixture/'+name)
        def start_backend():
            b=docker('run','-d','--pull=never','--label',label+'='+name,'--network',network,'--env-file',str(root/'backend.env'),'-p','127.0.0.1::8080',args.backend_image);created.append(b);i=owned('container',b);ports=i['NetworkSettings']['Ports']['8080/tcp'];assert len(ports)==1 and ports[0]['HostIp']=='127.0.0.1';Relay.backend=int(ports[0]['HostPort']);return b
        backend=start_backend();thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
        trust=ssl.create_default_context(cafile=str(pathlib.Path(args.ca) if args.ca else cert));jar=http.cookiejar.CookieJar();client=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=trust),urllib.request.HTTPCookieProcessor(jar))
        def request(path,body=None,method=None,headers=None,who=client):
            h={'Origin':origin};h.update(headers or {})
            if isinstance(body,dict):body=json.dumps(body).encode();h['Content-Type']='application/json'
            req=urllib.request.Request(origin+path,body,h,method=method)
            try:
                with who.open(req,timeout=15) as r:return r.status,r.read()
            except urllib.error.HTTPError as e:return e.code,e.read()
        def ready():
            for _ in range(60):
                try:
                    if request('/ready')[0]==200:return
                except (urllib.error.URLError,ConnectionError):pass
                time.sleep(.5)
            raise AssertionError('corrected backend readiness')
        ready();check(fingerprint()==before,'migration 11 to 12 preserves synthetic user/file records and role');check(sql('SELECT version FROM schema_migrations;')=='12','migration reaches version 12')
        # Exercise actual HTTP login, identity, privileged reads, logout and replay.
        check(request('/login',{'username':'admin','password':password})[0]==200,'real environment admin login')
        status,body=request('/me');me=json.loads(body);check(status==200 and me['username']=='admin' and me['is_admin'] is True,'real environment admin identity')
        check(request('/admin/files')[0]==200,'environment admin privileged read')
        check(request('/files',{'orig_name':'admin.txt','content_type':'text/plain','size_bytes':1})[0]==403,'environment identity cannot acquire database ownership')
        oldcookie=next(iter(jar)).value;check(request('/logout',{},method='POST')[0]==200,'real logout')
        anonymous=urllib.request.build_opener(urllib.request.ProxyHandler({}),urllib.request.HTTPSHandler(context=trust));check(request('/me',headers={'Cookie':'sfd_session='+oldcookie},who=anonymous)[0]==401,'real logout rejects replay')
        owned('container',backend);docker('rm','-f',backend);created.remove(backend);backend=start_backend();ready();check(request('/me',headers={'Cookie':'sfd_session='+oldcookie},who=anonymous)[0]==401,'logout revocation survives backend recreation')
        # Run the exact handler/security suite against this uniquely guarded database.
        pgports=pginfo['NetworkSettings']['Ports']['5432/tcp'];assert pgports[0]['HostIp']=='127.0.0.1'
        testenv=dict(os.environ,GOTOOLCHAIN='go1.26.9',SFD_AUTH_TEST_DSN='postgres://fixture:'+dbpass+'@127.0.0.1:'+pgports[0]['HostPort']+'/'+dbname+'?sslmode=disable',SFD_AUTH_TEST_OWNER=name)
        output=command(['go','test','-race','./internal/...','./cmd/...','-count=1','-v'],cwd=repo,env=testenv,timeout=180);(root/'go-tests.txt').write_text(output+'\n');check('--- PASS: TestAuthenticatedIdentities' in output and '--- SKIP:' not in output,'real PostgreSQL Go security regressions and race checks')
        # Separate ordinary browser/API account created only in the fixture.
        check(request('/register',{'email':'upload@example.invalid','username':'uploader','password':'Synthetic9-uploader'})[0]==201,'real registration')
        sql("UPDATE users SET email_verified=true WHERE username='uploader';")
        check(request('/login',{'username':'uploader','password':'Synthetic9-uploader'})[0]==200,'real registered-user login')
        status,body=request('/me');check(status==200 and json.loads(body)['is_admin'] is False,'registered identity readback')
        check(request('/admin/files')[0]==403,'registered account denied admin permission')
        data=b'Secure File Drop isolated native hash fixture\n';status,body=request('/files',{'orig_name':'fixture.txt','content_type':'text/plain','size_bytes':len(data)});check(status==201,'owned metadata creation');fid=json.loads(body)['id']
        boundary='sfd'+secrets.token_hex(12);multipart=('--'+boundary+'\r\nContent-Disposition: form-data; name="file"; filename="fixture.txt"\r\nContent-Type: text/plain\r\n\r\n').encode()+data+('\r\n--'+boundary+'--\r\n').encode()
        status,body=request('/upload?id='+fid,multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary});check(status==200,'real native upload and hash');digest=sql("SELECT sha256_hex FROM files WHERE id='"+fid+"';");check(digest==hashlib.sha256(data).hexdigest(),'native SHA-256 matches uploaded bytes')
        status,body=request('/links',{'id':fid,'ttl_seconds':60});check(status==200,'owned signed link');link=json.loads(body)['url'];req=urllib.request.Request(link)
        with anonymous.open(req,timeout=10) as r:check(r.status==200 and r.read()==data,'signed download bytes match')
        # Distinct account must not read, upload, link or delete the first account file.
        check(request('/register',{'email':'second@example.invalid','username':'second','password':'Synthetic9-second'})[0]==201,'second isolated account');sql("UPDATE users SET email_verified=true WHERE username='second';")
        check(request('/login',{'username':'second','password':'Synthetic9-second'})[0]==200,'second account login')
        check(request('/links',{'id':fid})[0]==404,'cross-account link denied');check(request('/upload?id='+fid,multipart,headers={'Content-Type':'multipart/form-data; boundary='+boundary})[0]==404,'cross-account upload denied');check(request('/user/files/'+fid,method='DELETE')[0]==404,'cross-account deletion denied')
        check(request('/user/files')[0]==200 and fid.encode() not in request('/user/files')[1],'file list isolates accounts')
        Relay.browser_html=(repo/'tests/auth/browser.html').read_text().replace('__FIXTURE__',json.dumps({'username':'admin','password':password}))

        if args.browser_module:
            # This trust setup is restricted to a disposable GitHub Actions runner.
            # It never runs on the Owner's Windows/WSL environment or changes its CA.
            check(os.environ.get('GITHUB_ACTIONS')=='true' and not args.cert,'ephemeral CI fixture trust only')
            nss=pathlib.Path.home()/'.pki/nssdb'
            check(not (nss/'cert9.db').exists(),'fresh CI browser trust database')
            nss.mkdir(parents=True,exist_ok=True)
            command(['certutil','-N','-d','sql:'+str(nss),'--empty-password'])
            command(['certutil','-A','-d','sql:'+str(nss),'-n',name,'-t','C,,','-i',str(cert)])
            browserenv=dict(os.environ,SFD_AUTH_BROWSER_MODULE=str(pathlib.Path(args.browser_module).resolve()))
            browserout=command(['node',str(repo/'tests/auth/browser.cjs'),origin],env=browserenv,timeout=60)
            (root/'browser-tests.txt').write_text(browserout+'\n')
            check('BROWSER ACCEPTANCE PASS:' in browserout,'real historical browser UI over normally validated HTTPS')
        (root/'windows-fixture.json').write_text(json.dumps({'Origin':origin,'Username':'admin','Password':password}))
        result['origin']=origin;result['backend_image']=args.backend_image;result['certificate_sha256']=hashlib.sha256(ssl.PEM_cert_to_DER_cert(cert.read_text())).hexdigest();(root/'ready.json').write_text(json.dumps({'origin':origin,'ready':True}))
        deadline=time.monotonic()+args.hold_seconds
        while time.monotonic()<deadline and not (root/'continue').exists():time.sleep(.5)
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
        result['existing_services_unchanged']=snapshot()==baseline;result['owned_cleanup_complete']=not docker('ps','-aq','--filter','label='+label+'='+name) and not docker('volume','ls','-q','--filter','label='+label+'='+name) and not docker('network','ls','-q','--filter','label='+label+'='+name)
        (root/'result.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result,indent=2))
    return 0 if not result['failures'] and result['existing_services_unchanged'] and result['owned_cleanup_complete'] else 1
if __name__=='__main__':raise SystemExit(main())