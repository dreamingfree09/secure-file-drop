"""Source-only manifest creation/verification. Never installs or starts a lab."""
import argparse,hashlib,json,pathlib,sys
HERE=pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
import guard
p=argparse.ArgumentParser();p.add_argument('--write',action='store_true');a=p.parse_args()
if a.write:
    rows=[]
    for f in sorted(HERE.rglob('*')):
        if not f.is_file() or f.name=='manifest.json' or '__pycache__' in f.parts or f.suffix=='.pyc':continue
        if f.is_symlink() or getattr(f.lstat(),'st_file_attributes',0)&0x400:raise guard.Stop('source-reparse')
        raw=f.read_bytes();rows.append({'path':f.relative_to(HERE).as_posix(),'bytes':len(raw),'SHA256':hashlib.sha256(raw).hexdigest()})
    (HERE/'manifest.json').write_bytes((json.dumps({'schema':'SFD_STAGE1_SOURCE_MANIFEST_V1','files':rows},indent=2)+'\n').encode())
print(json.dumps({'manifest_SHA256':guard.verify_manifest(HERE)}))
