"""Export disposable PUBLIC data only for memory-exclusive Windows chain validation."""
import json,pathlib,re,sys,time
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parent))
from fence import Stop,create_workspace,read_small,write_new,digest
from issuer_fixture import IssuerFixture,MemoryFixtureVault
from export_windows_fixture import windows_target

def export(target,cdp):
    target=windows_target(target)
    if not re.fullmatch(r"http://127\.0\.0\.1:[0-9]+/fixture-[0-9a-f-]{36}\.crl",cdp):raise Stop("memory-fixture-CDP")
    issuer=IssuerFixture(create_workspace(),MemoryFixtureVault()).create(cdp)
    r,l,c=issuer.public()
    foreign=IssuerFixture(create_workspace(),MemoryFixtureVault()).create(cdp,minimal=True)
    items={"root.der":r,"valid.der":l,"current.crl.der":c,"foreign-root.der":foreign.public()[0],"foreign.crl.der":foreign.public()[2],
           "corrupt.crl.der":c[:-1]+bytes([c[-1]^1])}
    for name in ("expired.der","revoked.der","client.der","expired.crl.der","wrong_aki.crl.der"):
        items[name]=read_small(issuer.root/name)
    for name,data in items.items():write_new(target/name,data)
    result={"scope":"public synthetic certificate/CRL data only","cdp":cdp,"private_keys_exported":False,
            "public_SHA256":{n:digest(x) for n,x in items.items()},
            "executed_context":json.loads(read_small(issuer.root/"source-context.json"))}
    write_new(target/"memory-export.json",json.dumps(result,indent=2).encode())
    print(json.dumps({"synthetic_public_export":True,"private_key_exported":False,"files":len(items)}))
if __name__=="__main__":
    if len(sys.argv)!=3:raise Stop("memory-fixture-export-args")
    export(*sys.argv[1:])
