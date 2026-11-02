"""Public upstream SBOM/advisory analysis; never runs storage or a bypass."""
import argparse,datetime,hashlib,json,pathlib,urllib.request,sys
HERE=pathlib.Path(__file__).resolve().parent

def get(url,body=None):
    req=urllib.request.Request(url,data=None if body is None else json.dumps(body).encode(),headers={'User-Agent':'SFD-defensive-public-advisory-review','Content-Type':'application/json'})
    with urllib.request.urlopen(req,timeout=30) as r:return r.read()

def audit(output):
    out=pathlib.Path(output);out.mkdir(exist_ok=False)
    lock=json.loads((HERE/'runtime-lock.json').read_text())['RustFS_SBOM'];raw=get(lock['URL'])
    assert hashlib.sha256(raw).hexdigest()==lock['SHA256'] and len(raw)==lock['bytes']
    (out/'upstream-sbom.json').write_bytes(raw);sbom=json.loads(raw)
    queries=[]
    for c in sbom.get('components',[]):
        if c.get('purl') and c.get('version'):
            queries.append({'package':{'purl':c['purl']}})
    assert queries
    findings=[];response_hashes=[]
    for start in range(0,len(queries),100):
        batch=queries[start:start+100];pending=list(enumerate(batch))
        for page in range(10):
            if not pending:break
            request=[q for _,q in pending];response=get('https://api.osv.dev/v1/querybatch',{'queries':request});response_hashes.append(hashlib.sha256(response).hexdigest());results=json.loads(response)['results'];assert len(results)==len(pending)
            following=[]
            for (index,q),r in zip(pending,results):
                for v in r.get('vulns',[]):findings.append({'purl':batch[index]['package']['purl'],'id':v['id'],'URL':'https://osv.dev/vulnerability/'+v['id']})
                if r.get('next_page_token'):following.append((index,dict(batch[index],page_token=r['next_page_token'])))
            pending=following
        assert not pending,'OSV pagination unresolved'
    unique={(x['purl'],x['id']):x for x in findings};result={'schema':'SFD_STAGE1_RUSTFS_ADVISORY_V1','UTC':datetime.datetime.now(datetime.timezone.utc).isoformat(),'upstream_SBOM_SHA256':lock['SHA256'],'queried_components':len(queries),'response_SHA256':response_hashes,'matches':list(unique.values()),'runtime_exploit_tested':False,'image_binary_SBOM_identity_proved':False,'independent_acceptance':False}
    (out/'result.json').write_bytes((json.dumps(result,indent=2)+'\n').encode());print(json.dumps(result))
    return result
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--output',required=True);a=p.parse_args()
    try:audit(a.output)
    except Exception as e:print(json.dumps({'status':'BLOCKED','kind':type(e).__name__}));sys.exit(1)
