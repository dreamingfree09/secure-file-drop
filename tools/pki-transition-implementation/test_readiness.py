import json,pathlib,sys,unittest,xml.etree.ElementTree as ET
HERE=pathlib.Path(__file__).resolve().parent;sys.path.insert(0,str(HERE))
from fence import Stop
from readiness import custody_candidate,startup_task_proposal
from approvals import fixture_approval,screen
class ReadinessTests(unittest.TestCase):
    def plan(self):
        return {"CA_credential_record":"SYNTHETIC vault A","backup_credential_record":"SYNTHETIC vault B",
                "copy_A":"SYNTHETIC site A","copy_B":"SYNTHETIC site B","recovered_credential_test":True,
                "archive_server_key_restore_test":True,"physical_separation_attested":False,"independent_review_sha256":"0"*64}
    def test_real_custody_cannot_be_inferred_from_fixture_restores(self):
        r=custody_candidate(self.plan());self.assertFalse(r["candidate_complete"]);self.assertFalse(r["real_creation_authorized"])
        self.assertIn("physical_separation_attested",r["missing"])
    def test_vault_and_copy_references_must_be_separate(self):
        p=self.plan();p["copy_B"]=p["copy_A"]
        with self.assertRaises(Stop):custody_candidate(p)
        p=self.plan();p["backup_credential_record"]=p["CA_credential_record"]
        with self.assertRaises(Stop):custody_candidate(p)
    def test_future_monitor_task_is_disabled_least_privilege_no_password(self):
        text=startup_task_proposal("S-1-5-21-1","SYNTHETIC-monitor.exe");t=ET.fromstring(text)
        self.assertEqual(t.findtext("./Settings/Enabled"),"false")
        self.assertEqual(t.findtext("./Principals/Principal/LogonType"),"InteractiveToken")
        self.assertEqual(t.findtext("./Principals/Principal/RunLevel"),"LeastPrivilege")
        self.assertNotIn("Password",text);self.assertNotIn("Docker",text)
    def test_four_consents_have_no_blanket_real_authority(self):
        for scope in ("create_issuer","install_current_user_root","select_endpoint","retire_old_root"):
            text=screen(scope,fixture_approval(scope));self.assertIn("PREPARATION",text)
        with self.assertRaises(Stop):screen("approve_all",{})
if __name__=="__main__":
    from context import pins
    execution_source=pins()
    r=unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(ReadinessTests))
    (HERE/"readiness-results.json").write_text(json.dumps({"tests_run":r.testsRun,"failures":len(r.failures),"errors":len(r.errors),"skips":len(r.skipped),"real_authority":False,"tasks_registered":0,"executed_source_SHA256":execution_source,"execution_source_unchanged":execution_source==pins()},indent=2)+"\n",newline="\n")
    raise SystemExit(0 if r.wasSuccessful() and execution_source==pins() else 1)
