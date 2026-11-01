"""CI packet contains PUBLIC synthetic certificate/CRL bytes only, never any key/archive."""
import json,pathlib,sys,time
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parent))
from fence import Stop,create_workspace,child,digest,read_small,write_new
from issuer_fixture import IssuerFixture,MemoryFixtureVault
def make():
    root=create_workspace();cdp="http://127.0.0.1:49152/fixture-"+root.name[-36:]+".crl"
    issuer=IssuerFixture(create_workspace(),MemoryFixtureVault()).create(cdp)
    foreign=IssuerFixture(create_workspace(),MemoryFixtureVault()).create(cdp,minimal=True)
    r,l,c=issuer.public()
    data={"root.der":r,"valid.der":l,"current.crl.der":c,"foreign-root.der":foreign.public()[0],
          "foreign.crl.der":foreign.public()[2],"corrupt.crl.der":c[:-1]+bytes([c[-1]^1])}
    for name in ("expired.der","revoked.der","client.der","expired.crl.der","wrong_aki.crl.der"):
        data[name]=read_small(issuer.root/name)
    for name,content in data.items():write_new(child(root,name),content)
    metadata={"schema":"SFD_PUBLIC_MEMORY_CHAIN_PACKET_V1","fixture_only":True,"cdp":cdp,
              "files":{name:{"sha256":digest(content),"bytes":len(content)} for name,content in data.items()},
              "private_keys_in_packet":False,"execution_context":json.loads(read_small(issuer.root/"source-context.json"))}
    write_new(child(root,"packet.json"),json.dumps(metadata,indent=2).encode())
    return root
if __name__=="__main__":
    print(str(make()))
