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
        cls.generation_regressions=[]
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
        self.assertIn("history/v1/"+self.root.name+"/source-context.used",data)
        for path in self.root.glob("*.old"):
            self.assertEqual(data["history/v1/"+self.root.name+"/"+path.name],read_small(path))
            self.assertNotIn(path.name,data)
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


    def history(self,root):
        from recovery_history import read_history
        return read_history(root)

    def second_archive(self,sign=False):
        root,result=self.restore_copy()
        first=IssuerFixture(root,self.ca)
        if sign:
            activate_fixture_recovery(root,prepare(root));first.new_crl("recovered")
        arc=create_workspace();receipt=snapshot(first,arc,self.backup.recovered())
        return root,result,arc,receipt

    def reseal(self,arc,m,data):
        """Only generated fixture data/credential: exercise semantic gates beyond MAC failure."""
        for name,value in data.items():
            sha=digest(value);p=arc/("object-"+sha)
            if not p.exists():write_new(p,value)
            m["files"][name]={"sha256":sha,"bytes":len(value)}
        encoded=canonical(m);(arc/"manifest.json").write_bytes(encoded)
        a=json.loads(read_small(arc/"authentication.json"));a["manifest_sha256"]=digest(encoded)
        header={k:v for k,v in a.items() if k!="tag"}
        key=hashlib.pbkdf2_hmac("sha256",self.backup.recovered(),DOMAIN+bytes.fromhex(a["salt"]),ITERATIONS)
        a["tag"]=hmac.new(key,canonical(header)+b"\0"+encoded,"sha256").hexdigest()
        (arc/"authentication.json").write_bytes(canonical(a))
        return digest(encoded)

    def assert_no_active_history(self,root):
        for name in ("source-context.json","source-context.used","recovery-context.json",
                     "recovery-context.used","recovered.started","recovered.crl.der","recovered.crl.pem"):
            self.assertFalse((root/name).exists(),name)

    def test_restore_snapshot_restore_retains_separate_versioned_history(self):
        first,r1,arc,receipt=self.second_archive()
        second=create_workspace()
        r2=restore(self.issuer,arc,second,self.ca.recovered(),self.backup.recovered(),receipt["manifest_sha256"])
        self.assert_no_active_history(second)
        before=self.history(first);after=self.history(second)
        self.assertTrue(all(after[n]==v for n,v in before.items()))
        self.assertEqual(after["history/v1/"+first.name+"/restore.started"],read_small(first/"restore.started"))
        self.assertEqual(after["history/v1/"+first.name+"/restore.verified.json"],canonical(r1))
        self.assertEqual(r2["generation"],2);self.assertNotEqual(r1["operation"],r2["operation"])
        self.assertEqual(r2["prior_receipt_sha256"],digest(canonical(r1)))
        self.assertEqual(r2["source_manifest_sha256"],receipt["manifest_sha256"])
        self.assertEqual(r2["source_fixture"],first.name);self.assertEqual(r2["target_fixture"],second.name)
        self.assertEqual(r2["next_serial"],r1["next_serial"])
        with self.assertRaises(Stop):IssuerFixture(second,self.ca).new_crl("recovered")
        self.__class__.generation_regressions.append({"case":"restore-snapshot-restore","generations":[1,2],
            "root_der_sha256":r2["root_der_sha256"],"history_objects":len(after),
            "active_operations_distinct":True,"all_prior_history_bytes_preserved":True})

    def test_two_recovery_generations_activate_and_sign_forward(self):
        first,r1=self.restore_copy();a=IssuerFixture(first,self.ca)
        lease1=prepare(first);activate_fixture_recovery(first,lease1)
        counter1=int(read_small(first/"crlnumber").strip(),16)
        a.new_crl("recovered");signed1=read_small(first/"recovered.crl.der")
        self.assertEqual(crl(signed1)["number"],counter1)
        arc=create_workspace();snap=snapshot(a,arc,self.backup.recovered())
        second=create_workspace();r2=restore(a,arc,second,self.ca.recovered(),self.backup.recovered(),snap["manifest_sha256"])
        self.assert_no_active_history(second)
        historical=self.history(second);baseline=dict(historical)
        prefix="history/v1/"+first.name+"/"
        for name in ("restore.started","restore.verified.json","recovery-context.json","recovery-context.used",
                     "recovered.started","recovered.crl.der","recovered.crl.pem"):
            self.assertEqual(historical[prefix+name],read_small(first/name))
        for path in first.glob("*.old"):
            self.assertEqual(historical[prefix+path.name],read_small(path))
            self.assertFalse((second/path.name).exists())
        self.assertEqual(r2["next_crl_counter"],counter1+1)
        self.assertEqual(read_small(first/"index.txt"),read_small(second/"index.txt"))
        self.assertEqual(read_small(first/"serial"),read_small(second/"serial"))
        b=IssuerFixture(second,self.ca);lease2=prepare(second)
        self.assertNotEqual(lease1["lease"],lease2["lease"])
        activate_fixture_recovery(second,lease2);b.new_crl("recovered")
        signed2=read_small(second/"recovered.crl.der");self.assertEqual(crl(signed2)["number"],counter1+1)
        root=b.command(["x509","-in","root.pem","-outform","DER"])
        validator=Validator(root,read_small(second/"valid.der"),self.cdp,OpenSSLVerifier(second))
        validator.validate(signed1,int(time.time()));validator.validate(signed2,int(time.time()))
        self.assertEqual(int(read_small(second/"crlnumber").strip(),16),counter1+2)
        self.assertEqual(historical,self.history(second))
        from der import certificate
        public_keys={}
        for role in ("valid","expired","revoked","client"):
            pub=b.command(["pkey","-in",role+".key","-pubout","-outform","DER"])
            self.assertEqual(pub,certificate(read_small(second/(role+".der")))["spki"])
            self.assertEqual(read_small(second/(role+".der")),read_small(self.root/(role+".der")))
            public_keys[role]=digest(pub)
        self.__class__.generation_regressions.append({"case":"restore-activate-sign-snapshot-restore-activate-sign",
            "generations":[1,2],"signed_CRL_numbers":[counter1,counter1+1],"final_next_CRL":counter1+2,
            "serial_preserved":True,"issued_ledger_preserved":True,"all_history_bytes_preserved":baseline==self.history(second),
            "root_der_sha256":digest(root),"server_public_key_SHA256":public_keys,
            "all_CRL_and_issued_certificate_signatures_verified":True,"history_objects":len(historical),
            "prior_receipt_sha256":r2["prior_receipt_sha256"],"fresh_target_bound_leases":True})

    def test_original_source_context_is_not_recovery_authority(self):
        root,_=self.restore_copy();old=json.loads(read_small(self.root/"source-context.json"))
        with self.assertRaises(Stop):activate_fixture_recovery(root,old)
        self.assertFalse((root/"recovery-context.json").exists())

    def test_previous_generation_recovery_context_rejected(self):
        first,_,arc,snap=self.second_archive(sign=True)
        second=create_workspace();restore(self.issuer,arc,second,self.ca.recovered(),self.backup.recovered(),snap["manifest_sha256"])
        old=json.loads(read_small(first/"recovery-context.json"))
        with self.assertRaises(Stop):activate_fixture_recovery(second,old)
        self.assertFalse((second/"recovery-context.json").exists())
        lease=prepare(second);activate_fixture_recovery(second,lease)
        used=read_small(second/"recovery-context.used")
        with self.assertRaises(FileExistsError):activate_fixture_recovery(second,lease)
        self.assertEqual(used,read_small(second/"recovery-context.used"))

    def test_tampered_historical_object_rejected_before_second_target_writes(self):
        _,_,arc,snap=self.second_archive(sign=True)
        m,data=open_snapshot(arc,self.backup.recovered(),snap["manifest_sha256"])
        name=next(n for n in data if n.endswith("/restore.verified.json"))
        (arc/("object-"+m["files"][name]["sha256"])).write_bytes(data[name]+b"\n")
        target=create_workspace()
        with self.assertRaises(Stop):restore(self.issuer,arc,target,self.ca.recovered(),self.backup.recovered(),snap["manifest_sha256"])
        self.assertEqual([p.name for p in target.iterdir()],["fixture.json"])

    def test_authenticated_wrong_history_identity_generation_or_counter_rejected(self):
        _,_,source,snap=self.second_archive(sign=True)
        original_m,original_data=open_snapshot(source,self.backup.recovered(),snap["manifest_sha256"])
        name=next(n for n in original_data if n.endswith("/provenance.json") and json.loads(original_data[n])["generation"]==1)
        for field,value in (("root_der_sha256","0"*64),("generation",0),("crl_next",1)):
            arc=create_workspace();copy_snapshot(source,arc);m=copy.deepcopy(original_m);data=dict(original_data)
            p=json.loads(data[name]);p[field]=value;data[name]=canonical(p)
            expected=self.reseal(arc,m,data);target=create_workspace()
            with self.assertRaises(Stop):restore(self.issuer,arc,target,self.ca.recovered(),self.backup.recovered(),expected)
            self.assertEqual([p.name for p in target.iterdir()],["fixture.json"])

    def test_authenticated_corrupt_historical_CRL_signature_rejected(self):
        arc=self.copied();m,data=open_snapshot(arc,self.backup.recovered(),self.receipt["manifest_sha256"])
        name=next(n for n in data if n.endswith("/current.crl.der"));bad=data[name][:-1]+bytes([data[name][-1]^1])
        from crypto import pem
        data[name]=bad;data[name[:-4]+".pem"]=pem(b"X509 CRL",bad)
        pn=name.rsplit("/",1)[0]+"/provenance.json";p=json.loads(data[pn])
        for n in (name,name[:-4]+".pem"):p["files"][n.rsplit("/",1)[1]]={"sha256":digest(data[n]),"bytes":len(data[n])}
        data[pn]=canonical(p);expected=self.reseal(arc,m,data);target=create_workspace()
        with self.assertRaises(Stop):restore(self.issuer,arc,target,self.ca.recovered(),self.backup.recovered(),expected)
        self.assertTrue((target/"restore.started").exists());self.assertFalse((target/"restore.verified.json").exists())

    def test_interrupted_second_restore_preserves_history_and_cannot_retry(self):
        first,_,arc,snap=self.second_archive(sign=True);target=create_workspace()
        with self.assertRaises(Stop):restore(self.issuer,arc,target,self.ca.recovered(),self.backup.recovered(),snap["manifest_sha256"],interrupt_after=2)
        saved={p.name:read_small(p) for p in target.iterdir() if p.is_file()}
        history=self.history(target)
        self.assertTrue(history);self.assertFalse((target/"restore.verified.json").exists())
        with self.assertRaises(Stop):restore(self.issuer,arc,target,self.ca.recovered(),self.backup.recovered(),snap["manifest_sha256"])
        self.assertEqual(saved,{p.name:read_small(p) for p in target.iterdir() if p.is_file()})
        self.assertEqual(history,self.history(target))
        with self.assertRaises(Stop):IssuerFixture(target,self.ca).new_crl("recovered")

    def test_no_active_receipt_or_context_can_be_overwritten(self):
        for name in ("restore.started","restore.verified.json","source-context.json","source-context.used",
                     "recovery-context.json","recovery-context.used"):
            target=create_workspace();sentinel=b"SYNTHETIC PRESERVE "+name.encode();write_new(target/name,sentinel)
            with self.assertRaises(Stop):restore(self.issuer,self.archive,target,self.ca.recovered(),self.backup.recovered(),self.receipt["manifest_sha256"])
            self.assertEqual(read_small(target/name),sentinel)
            self.assertEqual(sorted(p.name for p in target.iterdir()),sorted(["fixture.json",name]))

    def test_activation_does_not_overwrite_receipt_or_consumed_marker(self):
        root,r=self.restore_copy();receipt=read_small(root/"restore.verified.json")
        write_new(root/"recovery-context.used",b"SYNTHETIC consumed sentinel")
        with self.assertRaises(FileExistsError):activate_fixture_recovery(root,prepare(root))
        self.assertEqual(read_small(root/"restore.verified.json"),receipt)
        self.assertEqual(read_small(root/"recovery-context.used"),b"SYNTHETIC consumed sentinel")
        self.assertFalse((root/"recovery-context.json").exists())

    def test_current_receipt_tamper_rejected_before_forward_signing(self):
        root,r=self.restore_copy();activate_fixture_recovery(root,prepare(root))
        counter=read_small(root/"crlnumber");r["CA_and_server_public_identity_verified"]=False
        (root/"restore.verified.json").write_bytes(canonical(r))
        with self.assertRaises(Stop):IssuerFixture(root,self.ca).new_crl("recovered")
        self.assertEqual(read_small(root/"crlnumber"),counter)
        self.assertFalse((root/"recovered.started").exists())

    def test_resealed_archive_counter_downgrade_rejected(self):
        arc=self.copied();m,data=open_snapshot(arc,self.backup.recovered(),self.receipt["manifest_sha256"])
        for field in ("ledger_next_crl","ledger_next_serial"):
            changed=copy.deepcopy(m);changed[field]=1
            expected=self.reseal(arc,changed,data);target=create_workspace()
            with self.assertRaises(Stop):restore(self.issuer,arc,target,self.ca.recovered(),self.backup.recovered(),expected)
            self.assertEqual([p.name for p in target.iterdir()],["fixture.json"])

if __name__=="__main__":
    from context import pins
    execution_source=pins()
    result=unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(ArchiveTests))
    receipt={"scope":"complete disposable issuer/operation archive and encrypted server-key recovery",
             "tests_run":result.testsRun,"failures":len(result.failures),"errors":len(result.errors),"skips":len(result.skipped),
             "names":unittest.defaultTestLoader.getTestCaseNames(ArchiveTests),"real_CA_or_backups_accessed":False,
             "physical_custody_verified":False}
    if result.wasSuccessful():
        receipt["snapshot"]=ArchiveTests.receipt
        receipt["generation_regressions"]=ArchiveTests.generation_regressions
    receipt["executed_source_SHA256"]=execution_source
    receipt["execution_source_unchanged"]=execution_source==pins()
    (HERE/"archive-results.json").write_text(json.dumps(receipt,indent=2)+"\n",newline="\n")
    raise SystemExit(0 if result.wasSuccessful() and execution_source==pins() else 1)
