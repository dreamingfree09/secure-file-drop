"""Linux-only disposable issuer/custody rehearsal. No real-issuer entry point."""
import contextlib
import datetime
import json
import os
import pathlib
import secrets
import shutil
import stat
import subprocess
import time
import uuid
from fence import Stop, child, create_workspace, digest, owned, read_small, write_new, assert_openssl_toolchain
from publisher_fixture import publication_lock
from der import certificate
from context import bind_once

class MemoryFixtureVault:
    """Not a real escrow: disposable secret held separately in test-process memory."""
    def __init__(self):
        self.value=bytearray(secrets.token_urlsafe(32).encode("ascii"))
    def recovered(self):
        return bytes(self.value)

class IssuerFixture:
    def __init__(self, root, vault):
        if os.name=="nt":
            raise Stop("fixture-issuer-Linux-only")
        self.root=owned(root);self.vault=vault
        self.exe="/usr/bin/openssl"
        assert_openssl_toolchain()
        pins=json.loads((pathlib.Path(__file__).parent/"toolchain.json").read_text())
        if digest(pathlib.Path(self.exe).read_bytes())!=pins["openssl_sha256"]:
            raise Stop("fixture-tool-pin")

    def command(self, args, secret=None, data=None):
        owned(self.root)
        if args[0] not in ("ca","req","genpkey","pkey","x509","crl","verify"):
            raise Stop("fixture-command-role")
        if any(x.startswith(("pass:","env:","file:")) for x in args):
            raise Stop("fixture-secret-transport")
        for i,x in enumerate(args[:-1]):
            if x in ("-in","-key","-out","-config","-revoke","-trusted","-CRLfile","-CAfile"):
                value=pathlib.Path(args[i+1])
                value=child(owned(value.parent),value.name) if value.is_absolute() else child(self.root,str(value))
                if x=="-out" and value.exists():raise Stop("fixture-output-exists")
                if value.exists() and value.stat().st_nlink!=1:raise Stop("fixture-file-links")
        descriptors=()
        rd=None
        if secret is not None:
            if not isinstance(secret,bytes) or len(secret)>256 or b"\n" in secret:
                raise Stop("fixture-secret-shape")
            rd,wr=os.pipe()
            try:os.write(wr,secret+b"\n")
            finally:os.close(wr)
            descriptors=(rd,)
            args=[x.replace("SECRET_FD",str(rd)) for x in args]
        try:
            with publication_lock(self.root) if args[0]=="ca" else contextlib.nullcontext():
                p=subprocess.run([self.exe,*args],cwd=self.root,input=data,pass_fds=descriptors,
                             capture_output=True,timeout=60,env={"PATH":"/usr/bin:/bin","LANG":"C","OPENSSL_CONF":"/dev/null"})
        finally:
            if rd is not None:os.close(rd)
        if p.returncode:
            write_new(child(self.root,"diagnostic-"+str(uuid.uuid4())+".json"),
                      json.dumps({"operation":args[0],"exit_code":p.returncode,
                                  "stderr":p.stderr.decode("utf-8","replace")[:4000]}).encode())
            raise Stop("fixture-command-failed:"+args[0])
        return p.stdout

    def create(self, cdp, fail_after_key=False, minimal=False):
        owned(self.root)
        if not cdp.startswith("http://127.0.0.1:") or any(x in cdp for x in ("\n","\r","=")):
            raise Stop("fixture-CDP")
        write_new(child(self.root,"creation.started"),b"one-use disposable operation\n")
        bind_once(self.root)
        nonce=json.loads(read_small(self.root/"fixture.json"))["nonce"]
        conf="""[ca]
default_ca=issuer
[issuer]
dir=.
database=$dir/index.txt
new_certs_dir=$dir/newcerts
certificate=$dir/root.pem
private_key=$dir/ca.enc.pem
serial=$dir/serial
crlnumber=$dir/crlnumber
default_md=sha256
default_days=90
default_crl_days=7
policy=names
unique_subject=no
copy_extensions=none
x509_extensions=leaf_ext
crl_extensions=crl_ext
[names]
commonName=supplied
[req]
distinguished_name=dn
[dn]
[root_ext]
basicConstraints=critical,CA:true,pathlen:0
keyUsage=critical,keyCertSign,cRLSign
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid:always
[leaf_ext]
basicConstraints=critical,CA:false
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid:always
subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1
crlDistributionPoints=URI:CDP_VALUE
[client_ext]
basicConstraints=critical,CA:false
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=clientAuth
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid:always
subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1
crlDistributionPoints=URI:CDP_VALUE
[crl_ext]
authorityKeyIdentifier=keyid:always
[bad_crl]
2.5.29.35=DER:30:16:80:14:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00:00
"""
        write_new(child(self.root,"issuer.cnf"),conf.replace("CDP_VALUE",cdp).encode())
        (self.root/"newcerts").mkdir(mode=0o700)
        for name,data in (("index.txt",b""),("serial",(secrets.token_hex(16).upper()+"\n").encode()),("crlnumber",b"2000\n")):
            write_new(child(self.root,name),data)
        # Plain generated key travels only over a pipe into encrypted PKCS8.
        rd,wr=os.pipe()
        os.write(wr,self.vault.recovered()+b"\n");os.close(wr)
        fd=os.open(child(self.root,"ca.enc.pem"),os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
        gen=None
        try:
            gen=subprocess.Popen([self.exe,"genpkey","-algorithm","RSA","-pkeyopt","rsa_keygen_bits:3072"],
                                 stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,env={"PATH":"/usr/bin:/bin","OPENSSL_CONF":"/dev/null"})
            enc=subprocess.run([self.exe,"pkcs8","-topk8","-v2","aes-256-cbc","-v2prf","hmacWithSHA256",
                "-iter","600000","-passout","fd:"+str(rd)],stdin=gen.stdout,stdout=fd,
                stderr=subprocess.PIPE,pass_fds=(rd,),timeout=60,env={"PATH":"/usr/bin:/bin","OPENSSL_CONF":"/dev/null"})
            gen.stdout.close()
            if gen.wait(timeout=60)!=0 or enc.returncode!=0:raise Stop("fixture-key-encryption")
            os.fsync(fd)
        finally:
            os.close(rd);os.close(fd)
            if gen is not None and gen.poll() is None:
                gen.kill();gen.wait()
        if fail_after_key:raise Stop("injected-fixture-interruption")
        self.command(["req","-new","-x509","-key","ca.enc.pem","-passin","fd:SECRET_FD",
            "-config","issuer.cnf","-extensions","root_ext","-days","730","-sha256","-set_serial",
            str(int.from_bytes(secrets.token_bytes(16),"big") or 1),"-subj","/CN=SyntheticSFDFixture-"+nonce,
            "-out","root.pem"],self.vault.recovered())
        os.chmod(self.root/"root.pem",0o600)
        if minimal:
            self.issue("valid")
            self.new_crl("current")
            return self
        for kind in ("valid","expired","revoked","client"):
            self.issue(kind)
        self.command(["ca","-config","issuer.cnf","-revoke","revoked.pem","-passin","fd:SECRET_FD",
                      "-batch"],self.vault.recovered())
        self.new_crl("current")
        self.new_crl("expired",expired=True)
        self.new_crl("successor")
        self.new_crl("wrong_aki")
        return self

    def issue(self, kind):
        if kind not in ("valid","expired","revoked","client"):
            raise Stop("fixture-leaf-role")
        key=kind+".key";csr=kind+".csr";cert=kind+".pem"
        # Only synthetic unattended server keys are unencrypted.
        write_new(child(self.root,key),self.command(["genpkey","-algorithm","RSA","-pkeyopt","rsa_keygen_bits:3072"]))
        self.command(["req","-new","-key",key,"-subj","/CN=localhost","-out",csr])
        now=datetime.datetime.now(datetime.timezone.utc)
        start=now-datetime.timedelta(days=2 if kind=="expired" else 0,seconds=60)
        end=now+datetime.timedelta(days=-1 if kind=="expired" else 89)
        args=["ca","-config","issuer.cnf","-in",csr,"-out",cert,"-batch","-notext",
              "-startdate",start.strftime("%y%m%d%H%M%SZ"),"-enddate",end.strftime("%y%m%d%H%M%SZ"),
              "-extensions","client_ext" if kind=="client" else "leaf_ext","-passin","fd:SECRET_FD"]
        self.command(args,self.vault.recovered())
        for name in (csr,cert):os.chmod(self.root/name,0o600)
        write_new(child(self.root,kind+".der"),self.command(["x509","-in",cert,"-outform","DER"]))

    def new_crl(self, name, expired=False):
        if name not in ("current","expired","successor","wrong_aki"):
            raise Stop("fixture-crl-role")
        write_new(child(self.root,name+".started"),b"one-use fixture signing lock\n")
        args=["ca","-config","issuer.cnf","-gencrl","-passin","fd:SECRET_FD","-out",name+".crl.pem"]
        if name=="wrong_aki":
            args+=["-crlexts","bad_crl"]
        if expired:
            now=datetime.datetime.now(datetime.timezone.utc)
            deadline=now.replace(microsecond=0)+datetime.timedelta(seconds=1)
            args+=["-crl_lastupdate",now.strftime("%Y%m%d%H%M%SZ"),
                   "-crl_nextupdate",deadline.strftime("%Y%m%d%H%M%SZ")]
        self.command(args,self.vault.recovered())
        if expired:
            # Actual expiry of an otherwise conforming disposable signed CRL; no ledger/clock edits.
            time.sleep(max(0,deadline.timestamp()-time.time())+0.05)
        os.chmod(self.root/(name+".crl.pem"),0o600)
        write_new(child(self.root,name+".crl.der"),self.command(["crl","-in",name+".crl.pem","-outform","DER"]))

    def public(self):
        root=self.command(["x509","-in","root.pem","-outform","DER"])
        return root,read_small(self.root/"valid.der"),read_small(self.root/"current.crl.der")

    def recover_identity(self, key_path, secret):
        p=pathlib.Path(key_path)
        if p.parent!=owned(p.parent) or p.name!="ca.enc.pem":
            raise Stop("custody-key-scope")
        p=child(p.parent,p.name)
        st=p.stat()
        if st.st_nlink!=1 or stat.S_IMODE(st.st_mode)!=0o600 or st.st_uid!=os.getuid():
            raise Stop("custody-key-ACL")
        pub=self.command(["pkey","-in",str(p),"-passin","fd:SECRET_FD","-check","-noout"],secret)
        # OpenSSL key-check text is deliberately not logged; require command success then public match.
        return self.command(["pkey","-in",str(p),"-passin","fd:SECRET_FD","-pubout","-outform","DER"],secret)

    def backup(self, destination):
        destination=owned(destination)
        with publication_lock(self.root):
            names=("ca.enc.pem","root.pem","issuer.cnf","index.txt","index.txt.attr","serial","crlnumber")
            manifest={}
            for name in names:
                data=read_small(self.root/name);write_new(child(destination,name),data);manifest[name]=digest(data)
            encoded=json.dumps(manifest,sort_keys=True).encode()
            write_new(child(destination,"custody-manifest.json"),encoded)
            return digest(encoded)
    def restore(self, backup, target, expected_manifest, recovered_secret):
        backup,target=owned(backup),owned(target)
        data=read_small(backup/"custody-manifest.json")
        if digest(data)!=expected_manifest:raise Stop("custody-manifest")
        manifest=json.loads(data)
        expected={"ca.enc.pem","root.pem","issuer.cnf","index.txt","index.txt.attr","serial","crlnumber"}
        if set(manifest)!=expected:raise Stop("custody-inventory")
        for name in sorted(expected):
            data=read_small(backup/name)
            if digest(data)!=manifest[name]:raise Stop("custody-ciphertext-ledger-hash")
            write_new(child(target,name),data)
        pub=self.recover_identity(target/"ca.enc.pem",recovered_secret)
        root=self.command(["x509","-in",str(target/"root.pem"),"-outform","DER"])
        if pub!=certificate(root)["spki"]:raise Stop("custody-public-identity")
        return {"restored_files":len(expected),"public_identity_verified":True,
                "physical_second_location_verified":False,"real_vault_verified":False}
