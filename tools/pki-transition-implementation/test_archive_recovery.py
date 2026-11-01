"""Actual complete disposable issuer/server-key recovery tests. No real CA or backups."""
import copy
import hashlib
import hmac
import json
import os
import pathlib
import sys
import time
import unittest
sys.path.insert(0,str(pathlib.Path(__file__).resolve().parent))
from fence import Stop,create_workspace,read_small,digest,write_new,child
from issuer_fixture import IssuerFixture,MemoryFixtureVault
from archive_recovery import snapshot,restore,copy_snapshot,open_snapshot,canonical,ITERATIONS,DOMAIN,activate_fixture_recovery
from context import prepare
from crypto import Validator,OpenSSLVerifier
from der import crl

HERE=pathlib.Path(__file__).resolve().parent
class ArchiveTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root=create_workspace();cls.ca=MemoryFixtureVault();cls.backup=MemoryFixtureVault()
        cls.cdp="http://127.0.0.1:49152/fixture-"+cls.root.name[-36:]+".crl"
        cls.issuer=IssuerFixture(cls.root,cls.ca).create(cls.cdp)
        cls.archive=create_workspace()
        cls.receipt=snapshot(cls.issuer,cls.archive,cls.backup.recovered())

    def copied(self):
        target=create_workspace();copy_snapshot(self.archive,target);return target

    def restore_copy(self,archive=None,**kwargs):
        archive=archive or self.copied();target=create_workspace()
        result=restore(self.issuer,archive,target,self.ca.recovered(),self.backup.recovered(),
                       self.receipt["manifest_sha256"],**kwargs)
        return target,result

    def test_complete_snapshot_contains_archives_receipts_and_encrypted_server_keys(self):
        m,data=open_snapshot(self.archive,self.backup.recovered(),self.receipt["manifest_sha256"])
        self.assertEqual(len([x for x in data if x.startswith("newcerts/")]),4)
        self.assertEqual(len([x for x in data if x.startswith("encrypted-server-keys/")]),4)
        self.assertGreaterEqual(len([x for x in data if x.endswith(".crl.der")]),4)
        self.assertIn("source-context.used",data)
        self.assertTrue(all(not name.endswith(".key") for name in data))

    def test_full_restore_reconciles_all_public_private_identities(self):
        root,result=self.restore_copy()
        self.assertEqual(result["issued_certificates"],4)
        self.assertEqual(result["server_keys_restored"],4)
        self.assertTrue(result["CA_and_server_public_identity_verified"])
        self.assertEqual(read_small(root/"index.txt"),read_small(self.root/"index.txt"))
        self.assertEqual(read_small(root/"crlnumber"),read_small(self.root/"crlnumber"))
        self.assertFalse(result["real_vault_or_distinct_physical_media_verified"])

    def test_two_disjoint_copy_locations_restore_same_issuer(self):
        a,ra=self.restore_copy();b,rb=self.restore_copy()
        self.assertNotEqual(a,b);self.assertEqual(ra["root_der_sha256"],rb["root_der_sha256"])

    def test_restored_issuer_signs_forward_from_retained_counter(self):
        root,result=self.restore_copy()
        old=int(read_small(root/"crlnumber").strip(),16)
        candidate=IssuerFixture(root,self.ca)
        with self.assertRaises(Stop):candidate.new_crl("recovered")
        activate_fixture_recovery(root,prepare(root))
        candidate.new_crl("recovered")
        data=read_small(root/"recovered.crl.der")
        r=candidate.command(["x509","-in","root.pem","-outform","DER"])
        Validator(r,read_small(root/"valid.der"),self.cdp,OpenSSLVerifier(root)).validate(data,int(time.time()))
        self.assertEqual(crl(data)["number"],old)
        self.assertEqual(int(read_small(root/"crlnumber").strip(),16),old+1)

    def test_wrong_archive_credential_fails_before_restore_writes(self):
        root=create_workspace()
        with self.assertRaises(Stop):restore(self.issuer,self.archive,root,self.ca.recovered(),b"wrong-fixture-secret",self.receipt["manifest_sha256"])
        self.assertEqual([p.name for p in root.iterdir()],["fixture.json"])

    def test_wrong_CA_credential_stops_with_partial_state_preserved(self):
        root=create_workspace()
        with self.assertRaises(Stop):restore(self.issuer,self.archive,root,b"wrong-fixture-CA-secret",self.backup.recovered(),self.receipt["manifest_sha256"])
        self.assertTrue((root/"restore.started").exists())
        with self.assertRaises(Stop):restore(self.issuer,self.archive,root,self.ca.recovered(),self.backup.recovered(),self.receipt["manifest_sha256"])

    def test_ciphertext_object_tamper_stops_before_target_writes(self):
        arc=self.copied();m,_=open_snapshot(arc,self.backup.recovered(),self.receipt["manifest_sha256"])
        sha=m["files"]["encrypted-server-keys/valid.pem"]["sha256"];p=arc/("object-"+sha)
        p.write_bytes(read_small(p)+b"\n")
        root=create_workspace()
        with self.assertRaises(Stop):restore(self.issuer,arc,root,self.ca.recovered(),self.backup.recovered(),self.receipt["manifest_sha256"])
        self.assertEqual([p.name for p in root.iterdir()],["fixture.json"])

    def test_authentication_tag_or_KDF_tamper_fails(self):
        for field,value in (("tag","0"*64),("iterations",1),("salt","0"*32)):
            arc=self.copied();p=arc/"authentication.json";auth=json.loads(read_small(p));auth[field]=value;p.write_bytes(canonical(auth))
            with self.assertRaises(Stop):open_snapshot(arc,self.backup.recovered(),self.receipt["manifest_sha256"])

    def test_manifest_change_cannot_be_authorised_by_untrusted_hash(self):
        arc=self.copied();p=arc/"manifest.json";m=json.loads(read_small(p));m["ledger_next_crl"]=0x1002;p.write_bytes(canonical(m))
        with self.assertRaises(Stop):open_snapshot(arc,self.backup.recovered(),digest(read_small(p)))

    def test_missing_issued_certificate_fails_restore(self):
        arc=self.copied();m,_=open_snapshot(arc,self.backup.recovered(),self.receipt["manifest_sha256"])
        name=next(x for x in m["files"] if x.startswith("newcerts/"))
        # Do not delete: an authenticated inventory with a missing reference is not fabricated.
        p=arc/("object-"+m["files"][name]["sha256"]);p.rename(arc/"retained-missing-object")
        with self.assertRaises(OSError):self.restore_copy(arc)

    def test_snapshot_output_cannot_overwrite_an_existing_archive(self):
        with self.assertRaises(FileExistsError):snapshot(self.issuer,self.archive,self.backup.recovered())

    def test_restore_interruption_consumed_and_retained(self):
        root=create_workspace()
        with self.assertRaises(Stop):restore(self.issuer,self.archive,root,self.ca.recovered(),self.backup.recovered(),self.receipt["manifest_sha256"],interrupt_after=2)
        self.assertTrue((root/"restore.started").exists())
        with self.assertRaises(Stop):restore(self.issuer,self.archive,root,self.ca.recovered(),self.backup.recovered(),self.receipt["manifest_sha256"])

    def test_new_ledger_counter_not_downgraded(self):
        _,result=self.restore_copy()
        self.assertGreaterEqual(result["next_crl_counter"],0x2000)

if __name__=="__main__":
    from context import pins
    execution_source=pins()
    result=unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(ArchiveTests))
    receipt={"scope":"complete disposable issuer/operation archive and encrypted server-key recovery",
             "tests_run":result.testsRun,"failures":len(result.failures),"errors":len(result.errors),"skips":len(result.skipped),
             "names":unittest.defaultTestLoader.getTestCaseNames(ArchiveTests),"real_CA_or_backups_accessed":False,
             "physical_custody_verified":False}
    if result.wasSuccessful():receipt["snapshot"]=ArchiveTests.receipt
    receipt["executed_source_SHA256"]=execution_source
    receipt["execution_source_unchanged"]=execution_source==pins()
    (HERE/"archive-results.json").write_text(json.dumps(receipt,indent=2)+"\n",newline="\n")
    raise SystemExit(0 if result.wasSuccessful() and execution_source==pins() else 1)
