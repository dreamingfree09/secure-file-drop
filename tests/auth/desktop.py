#!/usr/bin/env python3
"""Actual unchanged portable client against a fresh owned schema12 HTTPS fixture.
No production configuration, credentials, trust store or existing Docker data is used.
"""
import argparse,datetime,hashlib,http.server,json,os,pathlib,re,socket,ssl,subprocess,threading,time

def main():
    parser=argparse.ArgumentParser()
    for name in ('backend-image','rollback-image','postgres-image','storage-image','proxy-image','output','client'):
        parser.add_argument('--'+name,required=True)
    parser.add_argument('--browser-module')
    parser.add_argument('--dotnet',default='dotnet')
    for name in ('cert','key','ca'):parser.add_argument('--'+name)
    args=parser.parse_args();os.umask(0o077)
    root=pathlib.Path(args.output);assert root.is_absolute();root.mkdir(mode=0o700,exist_ok=False)
    repo=pathlib.Path(__file__).resolve().parents[2];fixture=root/'fixture';untrusted=root/'untrusted';untrusted.mkdir(mode=0o700)
    subprocess.run(['openssl','req','-x509','-newkey','rsa:2048','-nodes','-days','1','-subj','/CN=localhost','-addext','subjectAltName=DNS:localhost','-keyout',str(untrusted/'key.pem'),'-out',str(untrusted/'cert.pem')],check=True,capture_output=True)
    class Untrusted(http.server.BaseHTTPRequestHandler):
        calls=0
        def log_message(self,*args):pass
        def do_GET(self):
            Untrusted.calls+=1;self.send_response(200);self.end_headers();self.wfile.write(b'{}')
    server=http.server.ThreadingHTTPServer(('127.0.0.1',0),Untrusted)
    context=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);context.load_cert_chain(untrusted/'cert.pem',untrusted/'key.pem');server.socket=context.wrap_socket(server.socket,server_side=True)
    thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
    unavailable=socket.socket();unavailable.bind(('127.0.0.1',0));unavailable.listen()
    def reject():
        while True:
            try:connection,_=unavailable.accept();connection.close()
            except OSError:return
    threading.Thread(target=reject,daemon=True).start()
    command=['python3','-B',str(repo/'tests/auth/acceptance.py'),'--output',str(fixture),'--hash-layout','bytea','--hold-seconds','300']
    for name in ('backend_image','rollback_image','postgres_image','storage_image','proxy_image'):
        command+=['--'+name.replace('_','-'),getattr(args,name)]
    if args.browser_module:command+=['--browser-module',args.browser_module]
    tls=[args.cert,args.key,args.ca];assert not any(tls) or all(tls),'all existing TLS paths required together'
    for name in ('cert','key','ca'):
        if getattr(args,name):command+=['--'+name,str(pathlib.Path(getattr(args,name)).resolve())]
    result={'source_revision':subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip(),'failures':[],'product_client_sha256':hashlib.sha256((repo/'desktop/windows/Client.cs').read_bytes()).hexdigest(),'Owner_visual_acceptance':False}
    process=None
    try:
        with (root/'private-fixture-output.txt').open('xb') as log:
            process=subprocess.Popen(command,cwd=repo,stdout=log,stderr=subprocess.STDOUT)
            try:
                result['stage']='owned-fixture-readiness'
                deadline=time.monotonic()+180
                while not (fixture/'ready.json').exists():
                    if process.poll() is not None or time.monotonic()>deadline:raise RuntimeError('owned schema12 fixture did not reach portable readiness')
                    time.sleep(.2)
                private=json.loads((fixture/'windows-fixture.json').read_text());owner=private['FixtureOwner'];assert re.fullmatch('sfd-auth-[0-9a-f]{12}',owner)
                assert private['Origin'].startswith('https://localhost:') and not private['Origin'].endswith(':8443')
                ids=subprocess.check_output(['docker','ps','-q','--filter','label=sfd.auth-fixture='+owner],text=True).split();assert ids
                for item in json.loads(subprocess.check_output(['docker','inspect',*ids],text=True)):
                    assert item['Config']['Labels'].get('sfd.auth-fixture')==owner
                private['UntrustedOrigin']='https://localhost:'+str(server.server_port)
                private['UnavailableOrigin']='http://127.0.0.1:'+str(unavailable.getsockname()[1])
                control=root/'private-portable-fixture.json';control.write_text(json.dumps(private))
                def runtime_path(path):
                    if args.dotnet.endswith('.exe'):return subprocess.check_output(['wslpath','-w',str(path)],text=True).strip()
                    return str(path)
                trust=pathlib.Path(args.ca).resolve() if args.ca else fixture/'fixture.crt'
                env=dict(os.environ,SSL_CERT_FILE=runtime_path(trust),SFD_DESKTOP_RUN=owner)
                if args.dotnet.endswith('.exe'):env['WSLENV']=env.get('WSLENV','')+':SFD_DESKTOP_RUN:SSL_CERT_FILE'
                # The browser and this client are independent legitimate journeys. They
                # otherwise combine eleven login/registration calls in the unchanged
                # production ten-per-minute window. Wait for that window to expire;
                # never raise/reset/bypass the limiter or remove client assertions.
                if args.browser_module:
                    result['stage']='waiting-for-normal-authentication-window'
                    start=time.monotonic();deadline=start+61
                    while time.monotonic()<deadline:time.sleep(.25)
                    result['auth_rate_window_wait_seconds']=round(time.monotonic()-start,3)
                result['stage']='portable-client'
                probe=subprocess.run([args.dotnet,runtime_path(pathlib.Path(args.client).resolve()),runtime_path(control)],env=env,cwd=repo,capture_output=True,text=True,timeout=120)
                (root/'private-portable-output.txt').write_text(probe.stdout+probe.stderr)
                result['portable_client_exit']=probe.returncode
                result['portable_passed_check_count']=probe.stdout.count('DESKTOP PASS:')
                kinds=re.findall(r'DESKTOP FAIL: ([A-Za-z][A-Za-z0-9]*)',probe.stderr)
                if kinds:result['portable_failure_kind']=kinds[-1]
                assert probe.returncode==0,'portable real-client execution failed; diagnostics preserved privately'
                assert probe.stdout.count('DESKTOP PASS:')==17 and 'DESKTOP REAL BACKEND CHECKS PASSED: 17' in probe.stdout
                assert Untrusted.calls==0,'untrusted TLS endpoint received an HTTP request'
                result.update(desktop_checks=17,portable_client_exit=0,untrusted_HTTP_requests=0,certificate_verification='normal .NET TLS with child-process-only fixture CA; no callback/store change')
            finally:
                if fixture.exists():
                    with (fixture/'continue').open('x') as signal:signal.write('continue')
                process.wait(timeout=180)
        result['stage']='completed-backend-and-owned-cleanup'
        actual=json.loads((fixture/'result.json').read_text())
        assert process.returncode==0 and not actual['failures'] and actual['existing_services_unchanged'] and actual['owned_cleanup_complete']
        assert actual['go_security']=={'passed_test_events':244,'skipped_test_events':0,'failed_test_events':0,'real_lineage_subtests':20,'real_lifecycle_ordering_cases':5,'real_commit_fault_modes':4}
        assert actual['rollback']['lockout_statuses']==[401]*5+[429]
        if args.browser_module:assert actual['browser']['count']==27
        result.update(backend_checks=len(actual['checks']),go_security=actual['go_security'],browser_checks=actual.get('browser',{}).get('count',0),owned_cleanup_complete=True,existing_services_unchanged=True)
    except (AssertionError,RuntimeError,subprocess.SubprocessError,OSError) as error:
        result['failures'].append(type(error).__name__)
    finally:
        server.shutdown();server.server_close();thread.join(timeout=5);unavailable.close()
        result['observed_utc']=datetime.datetime.now(datetime.timezone.utc).isoformat()
        (root/'desktop-identity.json').write_text(json.dumps(result,indent=2)+'\n')
    print(json.dumps(result,indent=2));return 1 if result['failures'] else 0
if __name__=='__main__':raise SystemExit(main())
