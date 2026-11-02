"""Create an exclusive offline review/staging bundle in a NEW Temp child only.
Never writes H, configures a host or extracts an executable. Missing media or
runtime evidence produces an incomplete bundle, never a fabricated GO.
"""
import argparse,hashlib,json,os,pathlib,shutil,sys,tempfile,uuid,zipfile
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parent))
import guard

def create(parent, inputs=None):
    parent=pathlib.Path(parent).resolve(strict=True)
    allowed=pathlib.Path(tempfile.gettempdir()).resolve(strict=True)
    if parent!=allowed:raise guard.Stop('only-new-Temp-review-staging')
    directory=parent/('sfd-stage1-prep-'+uuid.uuid4().hex)
    directory.mkdir(mode=0o700,exist_ok=False)
    guard.verify_manifest(guard.HERE)
    manifest=guard.strict_json((guard.HERE/'manifest.json').read_text(encoding='utf-8'))
    for row in manifest['files']:
        p=guard.HERE/row['path']; target=directory/row['path']
        target.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(p,target)
    shutil.copyfile(guard.HERE/'manifest.json',directory/'manifest.json')
    bundle={'schema':'SFD_STAGE1_INPUT_BUNDLE_V1','work_id':guard.POLICY['work_id'],
            'files':[], 'complete':False,'signed_media_verified':False,
            'windows_licence_verified':False,'runtime_acceptance_verified':False,
            'guest_network_payload_verified':False,
            'missing':['signed-linux-media','approved-Windows-media-and-rights',
                       'exact-runtime-export-and-accepted-CI','verified-offline-guest-payload']}
    if inputs:
        # An input declaration is staged for independent review, not promoted to
        # verified/live authority merely because a producer sets booleans.
        declaration=guard.strict_json(pathlib.Path(inputs).read_text(encoding='utf-8'))
        bundle['proposed_inputs']=declaration
    (directory/'bundle.json').write_bytes((json.dumps(bundle,indent=2)+'\n').encode())
    return {'directory':str(directory),'bundle_SHA256':hashlib.sha256((directory/'bundle.json').read_bytes()).hexdigest(),
            'installation_ready':False,'H_modified':False,'complete':False}
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--parent',default=tempfile.gettempdir());p.add_argument('--proposed-inputs');a=p.parse_args()
    try:print(json.dumps(create(a.parent,a.proposed_inputs)))
    except (guard.Stop,OSError) as e:print(json.dumps({'decision':'NO-GO','code':str(e)}));sys.exit(2)
