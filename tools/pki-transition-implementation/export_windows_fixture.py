"""Create a NEW Linux synthetic issuer; export only fixture public data and its disposable leaf key."""
import json
import pathlib
import re
import sys
import urllib.parse
import uuid
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parent))
from fence import SCHEMA,Stop,create_workspace,digest,read_small,write_new
from issuer_fixture import IssuerFixture,MemoryFixtureVault
from der import profiles

def windows_target(text):
    p=pathlib.Path(text)
    # Only a freshly marked Windows per-user Temp fixture; never an SFD directory.
    if not re.fullmatch(r"/mnt/c/Users/[^/]+/AppData/Local/Temp/sfd-pki-fixtures-[0-9a-f-]{36}",str(p)):
        raise Stop("Windows-fixture-export-path")
    if p.is_symlink():raise Stop("Windows-fixture-export-reparse")
    nonce=p.name[-36:]
    if str(uuid.UUID(nonce))!=nonce:raise Stop("Windows-fixture-export-nonce")
    if json.loads(read_small(p/"fixture.json"))!={"schema":SCHEMA,"nonce":nonce,"synthetic_only":True}:
        raise Stop("Windows-fixture-export-marker")
    return p

def export(public_text,private_text,cdp):
    public,private=windows_target(public_text),windows_target(private_text)
    if public==private:raise Stop("Windows-public-private-separation")
    url=urllib.parse.urlsplit(cdp)
    if url.scheme!="http" or url.hostname!="127.0.0.1" or url.port in (8443,18081,18082,18443,None) or url.query or url.fragment or url.username:
        raise Stop("Windows-fixture-CDP")
    if url.path!="/fixture-"+public.name[-36:]+".crl":raise Stop("Windows-fixture-CDP-path")
    issuer=IssuerFixture(create_workspace(),MemoryFixtureVault()).create(cdp)
    r,l,c=issuer.public()
    import time
    profiles(r,l,cdp,int(time.time()))
    items={"root.der":r,"valid.der":l,"current.crl.der":c}
    for name in ("expired.der","revoked.der","client.der","expired.crl.der","successor.crl.der","wrong_aki.crl.der"):
        items[name]=read_small(issuer.root/name)
    for name,data in items.items():write_new(public/name,data)
    # The CA key and secret NEVER leave the Linux fixture. Leaf key only goes into a separate ACL-protected TLS fixture.
    for name,source in (("leaf.pem","valid.pem"),("leaf.key","valid.key")):
        write_new(private/name,read_small(issuer.root/source))
    result={"schema":"SFD_SYNTHETIC_WINDOWS_EXPORT_V1","cdp":cdp,
            "public_SHA256":{n:digest(d) for n,d in items.items()},
            "CA_key_exported":False,"real_SFD_material_accessed":False,
            "executed_context":json.loads(read_small(issuer.root/"source-context.json")),
            "executed_context_sha256":digest(read_small(issuer.root/"source-context.json"))}
    write_new(public/"export.json",json.dumps(result,indent=2).encode())
    return result

if __name__=="__main__":
    if len(sys.argv)!=4:raise Stop("fixture-export-arguments")
    result=export(*sys.argv[1:])
    print(json.dumps({"synthetic_export":"PASS","public_files":len(result["public_SHA256"]),"CA_key_exported":False}))
