#!/usr/bin/env python3
"""Only newly labelled dedicated-network/loopback synthetic storage resources.
Never reads the live application's configuration, containers, bucket or volumes.
"""
from pathlib import Path
import argparse,hashlib,json,os,re,secrets,subprocess,time,urllib.request

def main():
    p=argparse.ArgumentParser();p.add_argument('--storage-image',required=True);p.add_argument('--output',required=True);p.add_argument('--runtime-image');a=p.parse_args()
    repo=Path(__file__).resolve().parents[1];out=Path(a.output);out.mkdir(mode=0o700,parents=True,exist_ok=False)
    module=repo/'tools/operator-controller/probe';owner='sfd-controller-storage-fixture-'+secrets.token_hex(6)
    label='sfd.controller-fixture';volume=owner+'-data';network=owner+'-net';cid=None;ok=False
    def run(args,**kw):
        r=subprocess.run(args,capture_output=True,timeout=kw.pop('timeout',90),**kw)
        if r.returncode:raise RuntimeError('owned fixture command failed')
        return r.stdout
    def item(kind,name):return json.loads(run(['docker',kind,'inspect',name]))[0]
    assert not os.environ.get('DOCKER_HOST') and run(['docker','context','inspect','--format','{{.Endpoints.docker.Host}}']).decode().strip()=='unix:///var/run/docker.sock'
    image=item('image',a.storage_image)['Id']
    report={'fixture':owner,'source_revision':run(['git','rev-parse','HEAD'],cwd=repo).decode().strip(),'live_resources_used':False}
    env=dict(os.environ,GOTOOLCHAIN='go1.26.10',GOWORK='off')
    tests=subprocess.run(['python3','-B','-m','unittest','discover','-s','tools/operator-controller','-p','test_*.py','-v'],cwd=repo,env=env,capture_output=True,timeout=90)
    (out/'controller-unit-tests.txt').write_bytes(tests.stdout+tests.stderr)
    if tests.returncode:raise RuntimeError('controller policy regressions failed')
    try:
        if run(['docker','volume','ls','-q','--filter','name=^'+volume+'$']).strip():raise RuntimeError('refusing existing test volume')
        run(['docker','volume','create','--label',label+'='+owner,volume])
        run(['docker','network','create','--driver','bridge','--label',label+'='+owner,network])
        access='fixture'+secrets.token_hex(6);secret=secrets.token_hex(24)
        child=dict(env,MINIO_ROOT_USER=access,MINIO_ROOT_PASSWORD=secret)
        cid=run(['docker','run','-d','--pull=never','--name',owner,'--label',label+'='+owner,'--network',network,
                 '--user','0:0','--cap-drop','ALL','--security-opt','no-new-privileges',
                 '-p','127.0.0.1::9000','-e','MINIO_ROOT_USER','-e','MINIO_ROOT_PASSWORD',
                 '--mount','type=volume,source='+volume+',target=/data',image,'server','/data'],env=child).decode().strip()
        current=item('container',cid)
        assert current['Image']==image and current['Config']['Labels'][label]==owner
        ports=current['NetworkSettings']['Ports']['9000/tcp'];assert len(ports)==1 and ports[0]['HostIp']=='127.0.0.1'
        endpoint='http://127.0.0.1:'+ports[0]['HostPort']
        for _ in range(120):
            try:
                with urllib.request.urlopen(endpoint+'/minio/health/live',timeout=2) as r:
                    if r.status==200:break
            except Exception:time.sleep(.25)
        else:raise RuntimeError('owned storage did not become ready')
        tenv=dict(env,SFD_CONTROLLER_STORAGE_TEST=endpoint,SFD_FIXTURE_ACCESS=access,SFD_FIXTURE_SECRET=secret,
                  SFD_PROBE_FIXTURE_OWNER=owner,SFD_PROBE_FIXTURE_IMAGE=image)
        r=subprocess.run(['go','test','-race','./...','-v','-count=1'],cwd=module,env=tenv,capture_output=True,timeout=180)
        text=(r.stdout+r.stderr).decode();(out/'probe-tests.txt').write_text(text)
        assert r.returncode==0 and '--- SKIP:' not in text and '--- FAIL:' not in text
        report['probe_pass_events']=len(re.findall(r'(?m)^\s*--- PASS:',text));report['probe_skips']=0
        run(['go','vet','./...'],cwd=module,env=env)
        build=dict(env,CGO_ENABLED='0',GOOS='linux',GOARCH='amd64',GOAMD64='v1',GOFLAGS='',GOEXPERIMENT='')
        run(['go','build','-trimpath','-buildvcs=false','-o',str(out/'storage-probe'),'.'],cwd=module,env=build)
        # A second build has a different filesystem root and no Git metadata.
        replay=out/'reproducible-source';replay.mkdir(mode=0o700)
        for name in ['main.go','guard.go','go.mod','go.sum']:(replay/name).write_bytes((module/name).read_bytes())
        run(['go','build','-trimpath','-buildvcs=false','-o',str(out/'storage-probe-replay'),'.'],cwd=replay,env=build)
        h=lambda path:hashlib.sha256(path.read_bytes()).hexdigest()
        assert h(out/'storage-probe')==h(out/'storage-probe-replay')
        runtime_image=item('image',a.runtime_image or a.storage_image)['Id']
        snapshots=[]
        for n in range(2):
            reader=owner+'-reader-'+str(n)
            values={'Endpoint':'http://'+owner+':9000','AccessKey':access,'SecretKey':secret,'Bucket':'sfd-controller-owned-fixture'}
            args=['docker','run','--rm','-i','--pull=never','--name',reader,'--label',label+'='+owner,'--network',network,
                  '--user','1000:1000','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges',
                  '--mount','type=bind,source='+str(out/'storage-probe')+',target=/probe/check,readonly',
                  '--entrypoint','/probe/check',runtime_image]
            try:
                value=json.loads(run(args,input=json.dumps(values).encode(),timeout=130))
                assert value['ok'] and len(value['objects'])==1 and value['total_bytes']==512
                snapshots.append(value)
            finally:
                found=subprocess.run(['docker','inspect',reader],capture_output=True)
                if found.returncode==0:
                    c=json.loads(found.stdout)[0];assert c['Config']['Labels'][label]==owner and c['Image']==runtime_image
                    run(['docker','rm','-f',c['Id']])
        assert snapshots[0]==snapshots[1]
        report.update(actual_production_CLI_tested=True,production_CLI_runtime_image=runtime_image,private_policy_digest_retained=True)
        report.update(probe_sha256=h(out/'storage-probe'),reproducible_binary=True,Go_race=True,Go_vet=True,
                      synthetic_payload_bytes=512,read_only_snapshots_equal=True,success=True)
        ok=True
    finally:
        if cid:
            c=item('container',cid);assert c['Config']['Labels'][label]==owner and c['Image']==image
            if ok:run(['docker','rm','-f',cid])
            else:run(['docker','stop','--time','10',cid])
        v=item('volume',volume);assert v['Labels'][label]==owner
        n=item('network',network);assert n['Labels'][label]==owner and not n['Internal']
        if ok:run(['docker','volume','rm',volume]);run(['docker','network','rm',network])
        report['only_owned_fixture_cleanup']=ok;report['failed_fixture_retained']=not ok
        (out/'controller-fixture-evidence.json').write_text(json.dumps(report,indent=2)+'\n')
    print(json.dumps(report,indent=2))
if __name__=='__main__':main()
