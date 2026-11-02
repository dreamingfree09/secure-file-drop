"""Existing GitHub-hosted ephemeral runner only. Never targets Owner Docker."""
import argparse,hashlib,json,os,pathlib,subprocess,sys,time
ROOT=pathlib.Path(__file__).resolve().parents[2]
LOCK=json.loads((ROOT/'tools/stage1/runtime-lock.json').read_text())

def run(args,cwd=None,env=None):
    p=subprocess.run(args,cwd=cwd or ROOT,env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
    if p.returncode:raise RuntimeError('owned-runtime-command-failed:'+pathlib.Path(args[0]).name)
    return p.stdout

def file_sha(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for chunk in iter(lambda:f.read(1024*1024),b''):h.update(chunk)
    return h.hexdigest()

def main():
    if os.name=='nt' or os.environ.get('GITHUB_ACTIONS')!='true' or os.environ.get('RUNNER_ENVIRONMENT')!='github-hosted' or not os.environ.get('RUNNER_TEMP') or 'microsoft' in pathlib.Path('/proc/sys/kernel/osrelease').read_text().lower() or not str(ROOT).startswith('/home/runner/work/secure-file-drop/'):
        raise RuntimeError('GitHub-ephemeral-runner-only')
    target=pathlib.Path(os.environ['RUNNER_TEMP'])/'sfd-stage1-runtime'
    target.mkdir(exist_ok=False)
    app=ROOT/'frozen-app';head=run(['git','rev-parse','HEAD'],app).decode().strip()
    assert head==LOCK['app_source']
    for role in ['postgres','traefik','rustfs']:run(['docker','pull',LOCK['images'][role]])
    run(['docker','build','-t','sfd-stage1-backend','-f',str(ROOT/'tools/stage1/Dockerfile.backend'),str(app)])
    run(['docker','build','-t','sfd-stage1-recovery','--label','sfd.recovery=artifact-only','-f',str(ROOT/'tools/stage1/Dockerfile.backend'),str(app)])
    run(['docker','build','-t','sfd-stage1-rustfs-fixture','-f','tools/stage1/Dockerfile.rustfs-fixture','.'])
    env=dict(os.environ);env.update(SFD_E2E='1',SFD_BROWSER='1',SFD_E2E_BACKEND_IMAGE='sfd-stage1-backend',SFD_E2E_ROLLBACK_IMAGE='sfd-stage1-recovery',SFD_E2E_POSTGRES_IMAGE=LOCK['images']['postgres'],SFD_E2E_STORAGE_IMAGE='sfd-stage1-rustfs-fixture',SFD_E2E_PROXY_IMAGE=LOCK['images']['traefik'],SFD_E2E_OUTPUT_ROOT=str(target),SFD_E2E_BROWSER_MODULE=str(ROOT/'frozen-app/tests/browser/node_modules/playwright-core'))
    run(['make','-C','native','test'])
    output=run(['go','test','-v','./tests/e2e','-run','^TestUploadHashDownloadFlow$','-count=1','-timeout','30m'],env=env)
    (target/'go-results.txt').write_bytes(output)
    suites=[]
    for name in ['sfd-auth-fixture','sfd-auth-fixture-bytea']:
        p=target/name/'result.json';r=json.loads(p.read_text());assert not r['failures'] and r['existing_services_unchanged'] and r['owned_cleanup_complete']
        assert len(r['checks'])>=91 and r['browser']['count']==27
        assert r['deletion_preservation']['admin_object_removed'] and r['deletion_preservation']['row_hash_unchanged'] and r['deletion_preservation']['storage_failure_status']==502
        assert r['go_security']['skipped_test_events']==0
        suites.append({'layout':r['hash_metadata_layout'],'checks':len(r['checks']),'browser':r['browser']['count'],'result_SHA256':hashlib.sha256(p.read_bytes()).hexdigest()})
    ids={}
    for name in ['sfd-stage1-backend','sfd-stage1-recovery',*[LOCK['images'][k] for k in ['postgres','traefik','rustfs']]]:
        ids[name]=json.loads(run(['docker','image','inspect',name]))[0]['Id']
    receipt={'schema':'SFD_STAGE1_RUNTIME_RECEIPT_V1','application':head,'Stage1_source':run(['git','rev-parse','HEAD']).decode().strip(),'candidate_locks':LOCK,'suites':suites,'exported_image_ids':ids,'functional_compatibility_passed':True,'full_security_acceptance':False,'limitations':['No host VM/network test','No stock Windows trusted-root/revocation/GUI test','Official OS-media signature/licence and RustFS full SBOM/advisory approval still required']}
    (target/'runtime-receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
    # Export official runtime bytes plus the exact new backend/recovery, not the
    # test-only adapter. Public images only; no containers/env/credentials.
    run(['docker','save','--output',str(target/'runtime-images.tar'),*sorted(set(ids.values()))])
    receipt['export_SHA256']=file_sha(target/'runtime-images.tar')
    (target/'runtime-receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
    print(json.dumps({'status':'PASS_REAL_RUNTIME_COMPATIBILITY','suites':suites,'security_acceptance':False}))
if __name__=='__main__':
    try:main()
    except Exception as e:print(json.dumps({'status':'FAIL','kind':type(e).__name__,'code':str(e) if isinstance(e,RuntimeError) else 'runtime-assertion'}));sys.exit(1)
