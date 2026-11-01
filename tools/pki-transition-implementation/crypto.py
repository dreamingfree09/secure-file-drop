"""Public-only signature validation. Windows uses Crypt32, never a permissive TLS callback."""
import base64
import ctypes
import json
import os
import pathlib
import subprocess
import uuid
from fence import Stop, digest, owned, write_new, assert_openssl_toolchain
from der import profiles, crl

def pem(label, data):
    b = base64.b64encode(data)
    return b"-----BEGIN "+label+b"-----\n"+b"\n".join(b[i:i+64] for i in range(0,len(b),64))+b"\n-----END "+label+b"-----\n"

class OpenSSLVerifier:
    def __init__(self, fixture_root):
        self.root = owned(fixture_root)
        pins = json.loads((pathlib.Path(__file__).parent/"toolchain.json").read_text())
        self.exe = "/usr/bin/openssl"
        assert_openssl_toolchain()
        if os.name == "nt" or digest(pathlib.Path(self.exe).read_bytes()) != pins["openssl_sha256"]:
            raise Stop("OpenSSL-tool-pin")

    def __call__(self, root, leaf, signed_crl):
        scratch = self.root / ("verify-" + str(uuid.uuid4()))
        scratch.mkdir(mode=0o700)
        for name, label, data in (("root.pem",b"CERTIFICATE",root),("leaf.pem",b"CERTIFICATE",leaf),("crl.pem",b"X509 CRL",signed_crl)):
            write_new(scratch/name, pem(label,data))
        p = subprocess.run([self.exe,"verify","-check_ss_sig","-no-CAfile","-no-CApath","-no-CAstore",
            "-trusted",str(scratch/"root.pem"),"-crl_check","-CRLfile",str(scratch/"crl.pem"),
            str(scratch/"leaf.pem")],capture_output=True,timeout=15,env={"PATH":"/usr/bin:/bin","LANG":"C","OPENSSL_CONF":"/dev/null"})
        if p.returncode != 0:
            raise Stop("OpenSSL-chain-signature-revocation")
        # Also demand explicit CRL verification success, not just process exit.
        p = subprocess.run([self.exe,"crl","-in",str(scratch/"crl.pem"),"-verify","-CAfile",
                            str(scratch/"root.pem"),"-noout"],capture_output=True,timeout=15,env={"PATH":"/usr/bin:/bin","LANG":"C","OPENSSL_CONF":"/dev/null"})
        if p.returncode != 0 or b"verify OK" not in (p.stdout+p.stderr).splitlines():
            raise Stop("OpenSSL-CRL-signature")

class WindowsVerifier:
    """Creates in-memory public contexts only; never opens/adds a certificate store."""
    def __init__(self):
        if os.name != "nt":
            raise Stop("Windows-required")
        c = ctypes.WinDLL("crypt32",use_last_error=True)
        ptr, u = ctypes.c_void_p, ctypes.c_uint32
        for name in ("CertCreateCertificateContext","CertCreateCRLContext"):
            fn=getattr(c,name);fn.argtypes=[u,ptr,u];fn.restype=ptr
        c.CryptVerifyCertificateSignatureEx.argtypes=[ptr,u,u,ptr,u,ptr,u,ptr]
        c.CryptVerifyCertificateSignatureEx.restype=ctypes.c_int
        c.CertFreeCertificateContext.argtypes=[ptr];c.CertFreeCertificateContext.restype=ctypes.c_int
        c.CertFreeCRLContext.argtypes=[ptr];c.CertFreeCRLContext.restype=ctypes.c_int
        self.c=c

    def __call__(self, root, leaf, signed_crl):
        c=self.c
        buffers=[ctypes.create_string_buffer(x) for x in (root,leaf,signed_crl)]
        contexts=[]
        try:
            for i,(data,buf) in enumerate(zip((root,leaf,signed_crl),buffers)):
                ctx=(c.CertCreateCRLContext if i==2 else c.CertCreateCertificateContext)(1,buf,len(data))
                if not ctx:raise Stop("Windows-public-context")
                contexts.append(ctx)
            for i,ctx in enumerate(contexts):
                if not c.CryptVerifyCertificateSignatureEx(None,1,3 if i==2 else 2,ctx,2,contexts[0],0,None):
                    raise Stop("Windows-public-signature")
        finally:
            for i,ctx in enumerate(contexts):
                (c.CertFreeCRLContext if i==2 else c.CertFreeCertificateContext)(ctx)

class Validator:
    def __init__(self, root, leaf, cdp, signature_verifier):
        self.root, self.leaf, self.cdp = root,leaf,cdp
        self.signature_verifier=signature_verifier
        self.identity={"root_sha256":digest(root),"leaf_sha256":digest(leaf),"cdp":cdp}

    def validate(self, data, now):
        r,l,ski=profiles(self.root,self.leaf,self.cdp,now)
        c=crl(data)
        if c["issuer"]!=r["subject"] or c["aki"]!=ski:
            raise Stop("CRL-issuer-AKI")
        if not c["start"]<=now<c["end"] or c["end"]-c["start"]>7*86400:
            raise Stop("CRL-validity")
        if l["serial"] in c["revoked"]:
            raise Stop("CRL-active-leaf-revoked")
        self.signature_verifier(self.root,self.leaf,data)
        return {"number":c["number"],"this_update":c["start"],"next_update":c["end"],
                "crl_sha256":digest(data),**self.identity}
