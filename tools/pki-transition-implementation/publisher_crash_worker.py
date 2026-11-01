"""Assignment-owned disposable public publisher process, deliberately killable at checkpoints."""
import json,os,pathlib,sys
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parent))
from fence import Stop,owned,child,read_small,write_new
from crypto import Validator,WindowsVerifier,OpenSSLVerifier
from publisher_fixture import Publication
def main():
    if len(sys.argv)!=3:raise Stop("crash-worker-arguments")
    root=owned(sys.argv[1]);phase=sys.argv[2]
    if phase not in ("after_bundle","after_water","after_pointer"):raise Stop("crash-worker-point")
    c=json.loads(read_small(root/"worker.json"))
    v=Validator(read_small(root/"root.der"),read_small(root/"valid.der"),c["cdp"],
                WindowsVerifier() if os.name=="nt" else OpenSSLVerifier(root))
    p=Publication(root,v)
    def checkpoint(name):
        if name==phase:
            write_new(child(root,"checkpoint-"+name+".json"),json.dumps({"pid":os.getpid(),"fixture_only":True}).encode())
            # Private anonymous stdin is a fixture barrier. No secret values or other process control.
            sys.stdin.buffer.read(1)
    p.publish(read_small(root/"successor.crl.der"),checkpoint=checkpoint)
if __name__=="__main__":main()
