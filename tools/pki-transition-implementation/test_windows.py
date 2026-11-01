"""Native Windows public CryptoAPI publisher and strict untrusted-root TLS fixture tests."""
import http.server
import json
import os
import pathlib
import socket
import ssl
import subprocess
import sys
import threading
import time
import unittest
import urllib.error
import urllib.request

HERE=pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
from fence import Stop,create_workspace,digest,read_small
from crypto import WindowsVerifier,Validator
from der import crl,certificate
from publisher_fixture import Publication,fixture_server,publication_lock
from approvals import fixture_approval,approve_real,screen

NATIVE=None
def native(*args):
    p=subprocess.run([str(NATIVE),*args],capture_output=True,text=True,timeout=35)
    if p.returncode:raise Stop("native-fixture-gate:"+args[0])
    return json.loads(p.stdout)

def wsl_path(p):
    return subprocess.check_output(["wsl.exe","-d","Ubuntu","--exec","wslpath","-a",str(p)],text=True).strip()

class WindowsPKITests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.store_before=native("--store-snapshot")
        cls.root,cls.tlsroot=create_workspace(),create_workspace()
        native("--secure-fixture-root",str(cls.root));native("--secure-fixture-root",str(cls.tlsroot))
        cls.server=fixture_server(cls.root)
        try:
            cmd=["wsl.exe","-d","Ubuntu","--exec","python3","-I","-B",
                 wsl_path(HERE/"export_windows_fixture.py"),wsl_path(cls.root),wsl_path(cls.tlsroot),cls.server.endpoint]
            p=subprocess.run(cmd,capture_output=True,text=True,timeout=120)
            if p.returncode:
                (cls.tlsroot/"export-failure.txt").write_text(p.stderr[:6000])
                raise Stop("synthetic-export-failed; private diagnostic retained")
            cls.r=read_small(cls.root/"root.der");cls.l=read_small(cls.root/"valid.der")
            cls.c=read_small(cls.root/"current.crl.der")
            # Root notBefore cannot be backdated with OpenSSL req3.0. Wait only for an already-signed
            # disposable fixture date to become current; no tolerance, clock edit or validation flag.
            due=max(certificate(cls.r)["not_before"],certificate(cls.l)["not_before"],crl(cls.c)["start"],crl(read_small(cls.root/"expired.crl.der"))["end"])
            delay=max(0,due-int(time.time()))
            if delay>10:raise Stop("fixture-host-clock-window")
            cls.fixture_date_wait_seconds=delay
            if delay:time.sleep(delay+0.1)
            cls.verifier=WindowsVerifier()
            cls.val=Validator(cls.r,cls.l,cls.server.endpoint,cls.verifier)
            cls.store=Publication(cls.root,cls.val);cls.store.publish(cls.c)
            cls.server.publication=cls.store
            cls.thread=threading.Thread(target=cls.server.serve_forever,daemon=True);cls.thread.start()
        except BaseException:
            cls.server.server_close()
            raise

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown();cls.server.server_close();cls.thread.join(timeout=5)
        cls.store_after=native("--store-snapshot")
        if cls.store_after!=cls.store_before:
            raise Stop("UNEXPECTED_WINDOWS_TRUST_SET_CHANGE")

    def test_actual_Windows_CryptoAPI_valid_signatures(self):
        self.verifier(self.r,self.l,self.c)
        self.assertEqual(self.val.validate(self.c,int(time.time()))["number"],0x2000)

    def test_CryptoAPI_signature_corruption_fails(self):
        with self.assertRaises(Stop):self.verifier(self.r,self.l,self.c[:-1]+bytes([self.c[-1]^1]))

    def test_CryptoAPI_wrong_root_signature_fails(self):
        # A changed public root signature is not a usable issuer context.
        with self.assertRaises(Stop):self.verifier(self.r[:-1]+bytes([self.r[-1]^1]),self.l,self.c)

    def test_revoked_leaf_rejected_with_actual_signed_CRL(self):
        v=Validator(self.r,read_small(self.root/"revoked.der"),self.server.endpoint,self.verifier)
        with self.assertRaises(Stop):v.validate(self.c,int(time.time()))

    def test_expired_leaf_rejected(self):
        v=Validator(self.r,read_small(self.root/"expired.der"),self.server.endpoint,self.verifier)
        with self.assertRaises(Stop):v.validate(self.c,int(time.time()))

    def test_client_EKU_rejected(self):
        v=Validator(self.r,read_small(self.root/"client.der"),self.server.endpoint,self.verifier)
        with self.assertRaises(Stop):v.validate(self.c,int(time.time()))

    def test_expired_signed_CRL_is_signature_valid_but_policy_rejected(self):
        data=read_small(self.root/"expired.crl.der")
        self.verifier(self.r,self.l,data)
        with self.assertRaises(Stop):self.val.validate(data,int(time.time()))

    def test_signed_wrong_AKI_rejected(self):
        data=read_small(self.root/"wrong_aki.crl.der")
        self.verifier(self.r,self.l,data)
        with self.assertRaises(Stop):self.val.validate(data,int(time.time()))

    def test_actual_public_DER_GET_and_HEAD(self):
        with urllib.request.urlopen(self.server.endpoint,timeout=3) as response:
            self.assertEqual(response.read(),self.c)
            self.assertEqual(response.headers["Content-Type"],"application/pkix-crl")
        with urllib.request.urlopen(urllib.request.Request(self.server.endpoint,method="HEAD"),timeout=3) as response:
            self.assertEqual(response.read(),b"")

    def test_query_traversal_and_mutation_rejected(self):
        for tail,method,code in (("?query","GET",404),("/../fixture.json","GET",404),("","POST",405),("","PUT",405)):
            with self.assertRaises(urllib.error.HTTPError) as caught:
                urllib.request.urlopen(urllib.request.Request(self.server.endpoint+tail,method=method),timeout=3)
            self.assertEqual(caught.exception.code,code)

    def test_unavailable_publisher_is_not_success(self):
        s=fixture_server(create_workspace());endpoint=s.endpoint;s.server_close()
        with self.assertRaises((urllib.error.URLError,TimeoutError)):urllib.request.urlopen(endpoint,timeout=2)

    def test_publication_gap_is_503(self):
        s=Publication(create_workspace(),self.val);s.publish(self.c)
        with self.assertRaises(Stop):s.publish(read_small(self.root/"successor.crl.der"),fault="after_water")
        server=fixture_server(s.root,s);t=threading.Thread(target=server.serve_forever,daemon=True);t.start()
        try:
            with self.assertRaises(urllib.error.HTTPError) as caught:urllib.request.urlopen(server.endpoint,timeout=3)
            self.assertEqual(caught.exception.code,503)
        finally:server.shutdown();server.server_close();t.join(timeout=3)

    def test_Windows_publication_lock_contention(self):
        root=create_workspace();p=Publication(root,self.val)
        with publication_lock(root):
            with self.assertRaises(Stop):p.publish(self.c)

    def test_cached_CRL_does_not_prove_new_publication(self):
        p=Publication(create_workspace(),self.val);p.publish(self.c);cached=p.read()
        with self.assertRaises(Stop):p.publish(read_small(self.root/"successor.crl.der"),fault="after_water")
        self.assertEqual(self.val.validate(cached,int(time.time()))["number"],0x2000)
        with self.assertRaises(Stop):p.read()

    def test_preview_forms_masked_and_non_authoritative(self):
        r=native("--self-test")
        self.assertTrue(r["MaskedInputDisabled"]);self.assertFalse(r["RealActionsImplemented"])
        with self.assertRaises(Stop):approve_real()

    def test_incorrect_trust_scope_rejected(self):
        i=fixture_approval("install_current_user_root")
        self.assertIn("Other applications",screen("install_current_user_root",i))
        i["store"]="LocalMachine/Root"
        with self.assertRaises(Stop):screen("install_current_user_root",i)
        with self.assertRaises(Stop):screen("LocalMachine/Root",i)

    def test_strict_Framework_and_Schannel_reject_untrusted_fixture(self):
        class Handler(http.server.BaseHTTPRequestHandler):
            def handle(self):
                try:super().handle()
                except ssl.SSLError as exc:
                    if exc.reason!="TLSV1_ALERT_UNKNOWN_CA":raise
                    self.server.expected_unknown_ca_alerts+=1
            def do_GET(self):
                self.send_response(200);self.end_headers();self.wfile.write(b"synthetic fixture")
            def log_message(self,*_):pass
        server=http.server.HTTPServer(("127.0.0.1",0),Handler)
        server.expected_unknown_ca_alerts=0
        ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        ctx.load_cert_chain(str(self.tlsroot/"leaf.pem"),str(self.tlsroot/"leaf.key"))
        server.socket=ctx.wrap_socket(server.socket,server_side=True)
        t=threading.Thread(target=server.serve_forever,daemon=True);t.start()
        uri="https://localhost:"+str(server.server_port)+"/health"
        try:
            result=native("--strict-reject",uri,str(self.root/"root.der"))
            self.assertTrue(result["HttpClientRejected"]);self.assertTrue(result["SslStreamRejected"])
            curl=subprocess.run(["curl.exe","--disable","--silent","--show-error","--max-time","10",uri],
                                capture_output=True,text=True,timeout=15)
            self.assertEqual(curl.returncode,60)
            self.__class__.strict_negative={"Framework":result,"Schannel_exit":curl.returncode,
                                           "origin":"owned ephemeral loopback","real_endpoint":False,"expected_unknown_CA_alerts":server.expected_unknown_ca_alerts}
        finally:server.shutdown();server.server_close();t.join(timeout=5)

if __name__=="__main__":
    if os.name!="nt" or len(sys.argv)!=2:raise Stop("native-Windows-fixture-tool-required")
    NATIVE=pathlib.Path(sys.argv.pop()).resolve()
    result=unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(WindowsPKITests))
    receipt={"scope":"native Windows disposable fixtures only; no real CA/trust changes",
        "tests_run":result.testsRun,"failures":len(result.failures),"errors":len(result.errors),"skips":len(result.skipped),
        "test_names":unittest.defaultTestLoader.getTestCaseNames(WindowsPKITests),
        "public_store_sets_unchanged":getattr(WindowsPKITests,"store_before",None)==getattr(WindowsPKITests,"store_after","missing"),
        "store_sets_before":getattr(WindowsPKITests,"store_before",None),
        "store_sets_after":getattr(WindowsPKITests,"store_after",None),
        "strict_negative":getattr(WindowsPKITests,"strict_negative",None),
        "fixture_binary_sha256":digest(NATIVE.read_bytes()),
        "fixture_date_wait_seconds":getattr(WindowsPKITests,"fixture_date_wait_seconds",None),
        "executed_context":json.loads(read_small(WindowsPKITests.root/"export.json"))["executed_context"] if hasattr(WindowsPKITests,"root") and (WindowsPKITests.root/"export.json").exists() else None,
        "positive_trusted_Windows_TLS_executed":False,"real_trust_changes":0,"Docker_operations":0}
    (HERE/"windows-results.json").write_text(json.dumps(receipt,indent=2)+"\n",newline="\n")
    raise SystemExit(0 if result.wasSuccessful() else 1)
