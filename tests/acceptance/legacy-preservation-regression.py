#!/usr/bin/env python3
"""Defensive deletion preservation regression using an exact inherited owned fixture.

No real DSN, bucket, credential or existing volume is accepted. The inherited
fixture creates fresh label-checked PostgreSQL/storage/backend resources and
removes only those resources. Failure is an actual failed acceptance gate, never
converted into a pass. See docs/LINEAGE_RECONCILIATION.md.
"""
import argparse, hashlib, json, os, pathlib, subprocess, sys

REVISION = "976c9a39c25f7cb38274bf066e5f7be05543813e"
FIXTURE_SHA256 = "ae45a6d79cc6b35f85b3f023dbd6089790369ea3bec3041843fd05bd7aa55a4a"
INSERTION = '        # Ordinary defensive preservation regression on an owned synthetic file.\n        # Administrator metadata removal must not depend on obsolete usernames.\n        jar.clear();check(request(\'/login\',{\'username\':\'admin\',\'password\':password})[0]==200,\'preservation probe admin login\')\n        admin_delete_status,_=request(\'/admin/files/\'+fid,method=\'DELETE\')\n        admin_metadata_present=sql("SELECT EXISTS(SELECT 1 FROM files WHERE id=\'"+fid+"\');")==\'t\'\n        r=subprocess.run([\'docker\',\'exec\',storage,\'mc\',\'stat\',\'fixture/\'+name+\'/uploads/\'+fid],capture_output=True)\n        admin_object_present=r.returncode==0\n        jar.clear();check(request(\'/login\',{\'username\':\'uploader\',\'password\':uploader_password})[0]==200,\'preservation probe owner login\')\n        # Create a distinct file: the admin probe may already have erased its row.\n        status,body=request(\'/files\',{\'orig_name\':\'storage-failure.txt\',\'content_type\':\'text/plain\',\'size_bytes\':len(data)})\n        check(status==201,\'separate storage-failure metadata creation\');fid=json.loads(body)[\'id\']\n        status,_=request(\'/upload?id=\'+fid,multipart,headers={\'Content-Type\':\'multipart/form-data; boundary=\'+boundary})\n        check(status==200,\'separate storage-failure file really uploaded\')\n        owned(\'container\',storage);docker(\'stop\',storage)\n        status,_=request(\'/user/files/\'+fid,method=\'DELETE\')\n        metadata_present=sql("SELECT EXISTS(SELECT 1 FROM files WHERE id=\'"+fid+"\');")==\'t\'\n        owned(\'container\',storage);docker(\'start\',storage)\n        for _ in range(60):\n            r=subprocess.run([\'docker\',\'exec\',storage,\'mc\',\'stat\',\'fixture/\'+name+\'/uploads/\'+fid],capture_output=True)\n            if r.returncode==0:break\n            time.sleep(.5)\n        object_present=r.returncode==0\n        result[\'preservation_regression\']={\'admin_delete_status\':admin_delete_status,\'admin_metadata_preserved\':admin_metadata_present,\'admin_object_present_after_success\':admin_object_present,\'owned_delete_storage_down_status\':status,\'metadata_preserved\':metadata_present,\'object_present_after_owned_storage_restart\':object_present,\'expected_storage_error_status\':502,\'expected_metadata_preserved\':True,\'target\':\'only newly created synthetic fixture\'}\n        check(status==502 and metadata_present and not admin_object_present,\'deletion must remove exact object and retain metadata on storage failure\')\n'

def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--legacy-checkout',required=True)
    parser.add_argument('--backend-image',required=True)
    parser.add_argument('--postgres-image',required=True)
    parser.add_argument('--storage-image',required=True)
    parser.add_argument('--output',required=True)
    args=parser.parse_args()
    os.umask(0o077)
    repo=pathlib.Path(args.legacy_checkout).resolve()
    def git(*items):return subprocess.check_output(['git','-C',str(repo),*items],text=True).strip()
    assert git('rev-parse','HEAD')==REVISION and not git('status','--porcelain'),'exact clean inherited fixture checkout required'
    inherited=subprocess.check_output(['git','-C',str(repo),'show',REVISION+':tests/auth/acceptance.py'])
    assert hashlib.sha256(inherited).hexdigest()==FIXTURE_SHA256,'fixture identity changed'
    for image in [args.backend_image,args.postgres_image,args.storage_image]:
        assert len(image)==71 and image.startswith('sha256:') and all(c in '0123456789abcdef' for c in image[7:]),'immutable local image IDs required'
    root=pathlib.Path(args.output).resolve();root.mkdir(mode=0o700,parents=False,exist_ok=False)
    marker='        # Distinct account must not read, upload, link or delete the first account file.'
    text=inherited.decode();assert text.count(marker)==1
    text=text.replace('repo=pathlib.Path(__file__).resolve().parents[2]','repo=pathlib.Path('+repr(str(repo))+')')
    text=text.replace(marker,INSERTION+marker)
    generated=root/'owned-fixture.py';generated.write_text(text)
    command=[sys.executable,'-B',str(generated),'--backend-image',args.backend_image,'--postgres-image',args.postgres_image,'--storage-image',args.storage_image,'--output',str(root/'private-fixture'),'--hash-layout','bytea']
    with (root/'private-runtime.txt').open('xb') as log:
        run=subprocess.run(command,stdout=log,stderr=subprocess.STDOUT,timeout=600)
    result=json.loads((root/'private-fixture/result.json').read_text())
    public={'inherited_revision':REVISION,'inherited_fixture_sha256':FIXTURE_SHA256,'probe_source_sha256':hashlib.sha256(pathlib.Path(__file__).read_bytes()).hexdigest(),'backend_image':args.backend_image,'exit':run.returncode,'actual':result.get('preservation_regression'),'failures':result.get('failures'),'existing_services_unchanged':result.get('existing_services_unchanged'),'owned_cleanup_complete':result.get('owned_cleanup_complete')}
    (root/'sanitized-result.json').write_text(json.dumps(public,indent=2)+'\n')
    print(json.dumps(public,indent=2))
    assert public['existing_services_unchanged'] and public['owned_cleanup_complete'],'preservation/cleanup failed'
    return run.returncode

if __name__=='__main__':sys.exit(main())
