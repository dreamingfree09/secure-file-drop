"""Windows-native public-only fixture publisher. Fixed loopback, ephemeral port; no live mode."""
import contextlib
import stat
import base64
import http.server
import json
import os
import re
import threading
import time
import uuid
from fence import Stop, child, digest, owned, read_small, write_new

@contextlib.contextmanager
def publication_lock(root):
    fd=os.open(child(root,"publisher.lock"),os.O_RDWR|os.O_CREAT|getattr(os,"O_NOFOLLOW",0),0o600)
    locked=False
    try:
        st=os.fstat(fd)
        if not stat.S_ISREG(st.st_mode) or st.st_nlink!=1:raise Stop("publisher-lock-file")
        if st.st_size==0:os.write(fd,b"0")
        os.lseek(fd,0,os.SEEK_SET)
        try:
            if os.name=="nt":
                import msvcrt
                msvcrt.locking(fd,msvcrt.LK_NBLCK,1)
            else:
                import fcntl
                fcntl.flock(fd,fcntl.LOCK_EX|fcntl.LOCK_NB)
        except OSError as exc:raise Stop("publisher-busy") from exc
        locked=True
        yield
    finally:
        if locked:
            if os.name=="nt":
                import msvcrt
                os.lseek(fd,0,os.SEEK_SET);msvcrt.locking(fd,msvcrt.LK_UNLCK,1)
            else:
                import fcntl
                fcntl.flock(fd,fcntl.LOCK_UN)
        os.close(fd)

def encoded(obj):
    return json.dumps(obj,sort_keys=True,separators=(",",":")).encode()

class Publication:
    def __init__(self, root, validator, min_number=0x2000):
        self.root=owned(root);self.validator=validator;self.floor=min_number
        self.lock=threading.RLock()
        self.seen=min_number-1

    def _replace(self, name, data):
        # Private immutable stage; a crash retains it. Only the owned pointer/highwater are replaced.
        target=child(self.root,name)
        stage=child(self.root,"stage-"+str(uuid.uuid4())+".json")
        write_new(stage,data)
        os.replace(stage,target)
        if os.name!="nt":
            fd=os.open(self.root,os.O_RDONLY|os.O_DIRECTORY)
            try:os.fsync(fd)
            finally:os.close(fd)

    def _water(self):
        p=child(self.root,"highwater.json")
        if not p.exists():return max(self.seen,self.floor-1)
        obj=json.loads(read_small(p))
        if set(obj)!={"number"} or type(obj["number"]) is not int or obj["number"]<self.floor:
            raise Stop("publication-water")
        return max(self.seen,obj["number"])

    def publish(self, data, now=None, fault=None):
        if now is None:now=int(time.time())
        with self.lock, publication_lock(self.root):
            info=self.validator.validate(data,now)
            high=self._water()
            if info["number"]<=high:raise Stop("publication-nonmonotonic")
            if info["next_update"]-now<86400:raise Stop("publication-margin")
            bundle={"schema":"SFD_PUBLIC_FIXTURE_BUNDLE_V1","identity":info,
                    "DER":base64.b64encode(data).decode("ascii")}
            blob=encoded(bundle);blobhash=digest(blob)
            name="bundle-"+str(info["number"])+"-"+blobhash+".json"
            write_new(child(self.root,name),blob)
            if fault=="after_bundle":raise Stop("injected-after-bundle")
            self._replace("highwater.json",encoded({"number":info["number"]}))
            self.seen=info["number"]
            if fault=="after_water":raise Stop("injected-after-water")
            self._replace("current.json",encoded({"bundle":name,"sha256":blobhash}))
            if fault=="after_pointer":raise Stop("injected-after-pointer")
            # Complete revalidation/readback; no success receipt can precede this.
            if self.read(now)!=data:raise Stop("publication-readback")
            return info

    def read(self, now=None):
        if now is None:now=int(time.time())
        with self.lock:
            pointer=json.loads(read_small(child(self.root,"current.json")))
            if set(pointer)!={"bundle","sha256"} or not re.fullmatch(r"bundle-[0-9]+-[0-9a-f]{64}\.json",pointer["bundle"]):
                raise Stop("publication-pointer")
            blob=read_small(child(self.root,pointer["bundle"]))
            if digest(blob)!=pointer["sha256"]:raise Stop("publication-bundle-hash")
            bundle=json.loads(blob)
            if set(bundle)!={"schema","identity","DER"} or bundle["schema"]!="SFD_PUBLIC_FIXTURE_BUNDLE_V1":
                raise Stop("publication-bundle-schema")
            data=base64.b64decode(bundle["DER"],validate=True)
            info=self.validator.validate(data,now)
            if info!=bundle["identity"] or info["number"]<self.floor or info["number"]!=self._water():
                raise Stop("publication-binding-water")
            self.seen=max(self.seen,info["number"])
            return data

def fixture_server(root, publication=None):
    root=owned(root)
    nonce=json.loads(read_small(root/"fixture.json"))["nonce"]
    exact_path="/fixture-"+nonce+".crl"
    class Handler(http.server.BaseHTTPRequestHandler):
        protocol_version="HTTP/1.0"
        def setup(self):
            super().setup()
            self.connection.settimeout(3)
        def log_message(self,*_):pass
        def answer(self,body=True):
            if self.path!=exact_path:
                self.send_error(404);return
            try:
                if self.server.publication is None:raise Stop("publisher-not-ready")
                data=self.server.publication.read()
            except (ValueError,OSError,KeyError,TypeError,Stop):
                self.send_response(503);self.send_header("Content-Length","0");self.end_headers();return
            self.send_response(200);self.send_header("Content-Type","application/pkix-crl")
            self.send_header("Content-Length",str(len(data)));self.send_header("Cache-Control","no-store")
            self.end_headers()
            if body:self.wfile.write(data)
        def do_GET(self):self.answer()
        def do_HEAD(self):self.answer(False)
        def do_POST(self):self.send_error(405)
        do_PUT=do_POST
        do_DELETE=do_POST
        do_OPTIONS=do_POST
    # Port0 only: this preparation cannot occupy old18081/new18082/live8443/preview18443.
    server=http.server.HTTPServer(("127.0.0.1",0),Handler)
    server.publication=publication
    server.exact_path=exact_path
    server.endpoint="http://127.0.0.1:"+str(server.server_port)+exact_path
    return server
