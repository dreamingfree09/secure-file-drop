"""Positive/negative revocation-enabled Windows chains with exclusive MEMORY roots, no system stores."""
import json,os,pathlib,subprocess,sys,threading,time,unittest,urllib.error,urllib.request
HERE=pathlib.Path(__file__).resolve().parent;sys.path.insert(0,str(HERE))
from fence import Stop,create_workspace,read_small,digest,write_new
from der import certificate,crl
from crypto import WindowsVerifier,Validator
from publisher_fixture import Publication,fixture_server
EXE=None
PUBLIC_PACKET=None
def ws(p):return subprocess.check_output(["wsl.exe","-d","Ubuntu","--exec","wslpath","-a",str(p)],text=True).strip()
class MemoryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root=create_workspace();cls.server=fixture_server(cls.root)
        if PUBLIC_PACKET is None:
            p=subprocess.run(["wsl.exe","-d","Ubuntu","--exec","python3","-I","-B",ws(HERE/"export_memory_fixture.py"),ws(cls.root),cls.server.endpoint],capture_output=True,text=True,timeout=120)
            if p.returncode:
                (cls.root/"export-error.txt").write_text(p.stderr)
                cls.server.server_close();raise Stop("memory-public-export-failed")
            cls.cdp=cls.server.endpoint
        else:
            metadata=json.loads(read_small(PUBLIC_PACKET/"packet.json"))
            if metadata.get("schema")!="SFD_PUBLIC_MEMORY_CHAIN_PACKET_V1" or metadata.get("fixture_only") is not True or metadata.get("private_keys_in_packet") is not False:
                raise Stop("memory-public-packet-schema")
            expected={"root.der","valid.der","current.crl.der","foreign-root.der","foreign.crl.der","corrupt.crl.der","expired.der","revoked.der","client.der","expired.crl.der","wrong_aki.crl.der"}
            if set(metadata["files"])!=expected:raise Stop("memory-public-packet-files")
            for name,item in metadata["files"].items():
                data=read_small(PUBLIC_PACKET/name)
                if len(data)!=item["bytes"] or digest(data)!=item["sha256"]:raise Stop("memory-public-packet-identity")
                write_new(cls.root/name,data)
            cls.cdp=metadata["cdp"]
        due=max(certificate(read_small(cls.root/"root.der"))["not_before"],
                certificate(read_small(cls.root/"foreign-root.der"))["not_before"],
                crl(read_small(cls.root/"current.crl.der"))["start"],
                crl(read_small(cls.root/"expired.crl.der"))["end"])
        delay=max(0,due-int(time.time()))
        if delay>10:raise Stop("fixture-clock-window")
        if delay:time.sleep(delay+0.1)
        cls.val=Validator(read_small(cls.root/"root.der"),read_small(cls.root/"valid.der"),cls.cdp,WindowsVerifier())
        cls.store=Publication(cls.root,cls.val);cls.store.publish(read_small(cls.root/"current.crl.der"))
        cls.server.publication=cls.store;cls.thread=threading.Thread(target=cls.server.serve_forever,daemon=True);cls.thread.start()
    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown();cls.server.server_close();cls.thread.join(timeout=5)
    def check(self,root="root.der",leaf="valid.der",signed="current.crl.der",host="localhost"):
        p=subprocess.run([str(EXE),"--check",str(self.root),root,leaf,signed,host],capture_output=True,text=True,timeout=10)
        self.assertEqual(p.returncode,0,p.stderr)
        return json.loads(p.stdout)
    def test_positive_exclusive_memory_root_and_current_CRL(self):
        r=self.check();self.assertTrue(r["trusted"]);self.assertTrue(r["revocation_required"])
        self.assertFalse(r["system_store_provider_used"]);self.assertFalse(r["real_trust_installed"])
    def test_missing_CRL_rejected(self):
        self.assertFalse(self.check(signed="none")["trusted"])
    def test_revoked_leaf_rejected_by_native_chain(self):
        r=self.check(leaf="revoked.der");self.assertFalse(r["trusted"]);self.assertNotEqual(r["chain_error"]&4,0)
    def test_expired_leaf_rejected(self):
        self.assertFalse(self.check(leaf="expired.der")["trusted"])
    def test_expired_CRL_rejected(self):
        self.assertFalse(self.check(signed="expired.crl.der")["trusted"])
    def test_corrupt_CRL_rejected(self):
        self.assertFalse(self.check(signed="corrupt.crl.der")["trusted"])
    def test_wrong_AKI_CRL_rejected(self):
        self.assertFalse(self.check(signed="wrong_aki.crl.der")["trusted"])
    def test_wrong_issuer_CRL_rejected(self):
        self.assertFalse(self.check(signed="foreign.crl.der")["trusted"])
    def test_wrong_exclusive_root_not_satisfied_by_any_other_store(self):
        self.assertFalse(self.check(root="foreign-root.der")["trusted"])
    def test_wrong_hostname_and_EKU_rejected(self):
        self.assertFalse(self.check(host="wrong.invalid")["trusted"])
        self.assertFalse(self.check(leaf="client.der")["trusted"])
    def test_public_publisher_readback_precedes_memory_validation(self):
        with urllib.request.urlopen(self.server.endpoint,timeout=3) as response:
            self.assertEqual(response.read(),read_small(self.root/"current.crl.der"))
        self.assertTrue(self.check()["trusted"])
    def test_no_claim_that_warm_memory_CRL_proves_live_publisher(self):
        s=fixture_server(create_workspace());url=s.endpoint;s.server_close()
        with self.assertRaises((urllib.error.URLError,TimeoutError)):urllib.request.urlopen(url,timeout=2)
        self.assertTrue(self.check()["trusted"])
if __name__=="__main__":
    from context import pins
    execution_source=pins()
    if os.name!="nt" or len(sys.argv) not in (2,3):raise Stop("Windows-memory-probe-required")
    EXE=pathlib.Path(sys.argv[1]).resolve()
    if len(sys.argv)==3:PUBLIC_PACKET=pathlib.Path(sys.argv[2]).resolve()
    result=unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(MemoryTests))
    receipt={"scope":"exclusive in-memory Windows trust; not default Schannel/Framework TLS",
       "tests_run":result.testsRun,"failures":len(result.failures),"errors":len(result.errors),"skips":len(result.skipped),
       "names":unittest.defaultTestLoader.getTestCaseNames(MemoryTests),
       "application_system_store_provider_calls":0,"real_trust_entries_installed":0,
       "OS_chain_internal_metadata_access":"not instrumented; exclusive fixture trust anchors only",
       "public_packet_mode":PUBLIC_PACKET is not None,"default_Framework_F196_positive_executed":False,
       "CRL_network_retrieval_by_native_chain":False,"revocation_checks_required":True}
    receipt["executed_source_SHA256"]=execution_source
    receipt["execution_source_unchanged"]=execution_source==pins()
    (HERE/"memory-chain-results.json").write_text(json.dumps(receipt,indent=2)+"\n",newline="\n")
    raise SystemExit(0 if result.wasSuccessful() and execution_source==pins() else 1)
