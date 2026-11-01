"""Abrupt child-process death and forward reconciliation using only new public fixture data."""
import json,os,pathlib,subprocess,sys,time,unittest
HERE=pathlib.Path(__file__).resolve().parent;sys.path.insert(0,str(HERE))
from fence import Stop,child,create_workspace,digest,read_small,write_new
from issuer_fixture import IssuerFixture,MemoryFixtureVault
from crypto import Validator,OpenSSLVerifier
from publisher_fixture import Publication
from publisher_recovery import reconcile_fixture,monitor_fixture
from context import prepare

class RecoveryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.issuer=IssuerFixture(create_workspace(),MemoryFixtureVault()).create("http://127.0.0.1:49152/fixture-recovery.crl")
        cls.r,cls.l,cls.c=cls.issuer.public();cls.cdp="http://127.0.0.1:49152/fixture-recovery.crl"
        cls.next=read_small(cls.issuer.root/"successor.crl.der")
    def setup_publication(self):
        root=create_workspace()
        for name,data in (("root.der",self.r),("valid.der",self.l),("successor.crl.der",self.next)):
            write_new(child(root,name),data)
        write_new(child(root,"worker.json"),json.dumps({"cdp":self.cdp}).encode())
        v=Validator(self.r,self.l,self.cdp,OpenSSLVerifier(root));p=Publication(root,v);p.publish(self.c)
        return root,v,p
    def crash(self,point):
        root,v,p=self.setup_publication()
        childproc=subprocess.Popen([sys.executable,"-I","-B",str(HERE/"publisher_crash_worker.py"),str(root),point],
             stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
        marker=root/("checkpoint-"+point+".json")
        try:
            end=time.monotonic()+15
            while not marker.exists() and time.monotonic()<end and childproc.poll() is None:time.sleep(0.03)
            self.assertTrue(marker.exists(),"owned child did not reach crash checkpoint")
            self.assertEqual(json.loads(read_small(marker))["pid"],childproc.pid)
            # Stop only this verified Popen-owned child, never a name-based/system process.
            childproc.kill();childproc.wait(timeout=5)
            self.assertNotEqual(childproc.returncode,0)
        finally:
            if childproc.poll() is None:childproc.kill();childproc.wait(timeout=5)
            if childproc.stdin:childproc.stdin.close()
            if childproc.stdout:childproc.stdout.close()
            if childproc.stderr:childproc.stderr.close()
        candidates=[x for x in root.glob("bundle-*.json") if json.loads(read_small(x))["identity"]["number"]>0x2000]
        self.assertEqual(len(candidates),1)
        return root,v,p,digest(read_small(candidates[0]))

    def test_process_death_after_bundle_preserves_old_selection(self):
        root,v,p,sha=self.crash("after_bundle");self.assertEqual(p.read(),self.c)
        with self.assertRaises(Stop):reconcile_fixture(root,v,sha,0x2000,prepare(root))
        r=reconcile_fixture(root,v,sha,0x2000,prepare(root),allow_uncommitted=True)
        self.assertGreater(r["published_number"],0x2000);self.assertFalse(r["counter_downgraded"])

    def test_process_death_after_water_is_stop_then_forward_recovery(self):
        root,v,p,sha=self.crash("after_water")
        self.assertEqual(monitor_fixture(p)["status"],"STOP")
        water=json.loads(read_small(root/"highwater.json"))["number"]
        lease=prepare(root);r=reconcile_fixture(root,v,sha,water,lease)
        self.assertEqual(r["published_number"],water)
        self.assertEqual(Publication(root,v).read(),self.next)
        with self.assertRaises(FileExistsError):reconcile_fixture(root,v,sha,water,lease)

    def test_process_death_after_pointer_preserves_complete_binding(self):
        root,v,p,sha=self.crash("after_pointer")
        self.assertEqual(p.read(),self.next)
        water=json.loads(read_small(root/"highwater.json"))["number"]
        self.assertEqual(reconcile_fixture(root,v,sha,water,prepare(root))["published_number"],water)

    def test_wrong_reviewed_counter_or_bundle_never_repaired(self):
        root,v,p,sha=self.crash("after_water")
        with self.assertRaises(Stop):reconcile_fixture(root,v,sha,0x1002,prepare(root))
        with self.assertRaises(Stop):reconcile_fixture(root,v,"0"*64,0x2002,prepare(root))
        self.assertEqual(monitor_fixture(p)["status"],"STOP")

    def test_corrupt_bundle_cannot_be_reconciled(self):
        root,v,p,sha=self.crash("after_water")
        for path in root.glob("bundle-*.json"):
            if digest(read_small(path))==sha:path.write_bytes(read_small(path)+b" ")
        with self.assertRaises(Stop):reconcile_fixture(root,v,sha,0x2002,prepare(root))
        self.assertEqual(monitor_fixture(p)["status"],"STOP")

    def test_no_highwater_rollback_to_old_valid_bundle(self):
        root,v,p,sha=self.crash("after_water")
        old=next(x for x in root.glob("bundle-*.json") if json.loads(read_small(x))["identity"]["number"]==0x2000)
        with self.assertRaises(Stop):reconcile_fixture(root,v,digest(read_small(old)),0x2002,prepare(root))

if __name__=="__main__":
    from context import pins
    execution_source=pins()
    r=unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(RecoveryTests))
    receipt={"scope":"abrupt verified-owned process death, not hardware power-loss proof","tests_run":r.testsRun,
             "failures":len(r.failures),"errors":len(r.errors),"skips":len(r.skipped),
             "names":unittest.defaultTestLoader.getTestCaseNames(RecoveryTests),"unrelated_processes_stopped":0,
             "real_publisher_or_backups_accessed":False,"power_failure_executed":False}
    receipt["executed_source_SHA256"]=execution_source
    receipt["execution_source_unchanged"]=execution_source==pins()
    (HERE/"publisher-recovery-results.json").write_text(json.dumps(receipt,indent=2)+"\n",newline="\n")
    raise SystemExit(0 if r.wasSuccessful() and execution_source==pins() else 1)
