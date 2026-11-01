"""Actual disposable OpenSSL issuer/CRL/custody/publisher tests; never uses old SFD material."""
import base64
import datetime
import copy
import hashlib
import importlib.util
import json
import os
import pathlib
import socket
import sys
import threading
import time
import unittest
import urllib.error
import urllib.request

HERE=pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
from fence import Stop, child, create_workspace, digest, owned, read_small, real_transition
from crypto import OpenSSLVerifier, Validator
from der import certificate, crl, nodes, seq, integer, oid
from context import prepare,verify,bind_once
from lifecycle import renewal_plan
from issuer_fixture import IssuerFixture, MemoryFixtureVault
from publisher_fixture import Publication, fixture_server, publication_lock
from approvals import fixture_approval, approve_real, screen

class PKITests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.root=create_workspace();cls.vault=MemoryFixtureVault()
        cls.cdp="http://127.0.0.1:49152/fixture-"+cls.root.name[-36:]+".crl"
        cls.issuer=IssuerFixture(cls.root,cls.vault).create(cls.cdp)
        cls.r,cls.l,cls.c=cls.issuer.public()
        cls.verifier=OpenSSLVerifier(cls.root)
        cls.val=Validator(cls.r,cls.l,cls.cdp,cls.verifier)
        cls.foreign=IssuerFixture(create_workspace(),MemoryFixtureVault()).create(cls.cdp,minimal=True)
        cls.now=int(time.time())

    def test_actual_current_chain_and_CRL(self):
        result=self.val.validate(self.c,self.now)
        self.assertEqual(result["number"],0x2000)

    def test_root_and_leaf_keys_are_independent(self):
        self.assertNotEqual(certificate(self.r)["spki"],certificate(self.l)["spki"])

    def test_expired_certificate_rejected_by_profile_and_OpenSSL(self):
        bad=read_small(self.root/"expired.der")
        with self.assertRaises(Stop):Validator(self.r,bad,self.cdp,self.verifier).validate(self.c,self.now)
        with self.assertRaises(Stop):self.issuer.command(["verify","-trusted","root.pem","-purpose","sslserver","expired.pem"])

    def test_revoked_certificate_rejected_by_profile_and_OpenSSL(self):
        bad=read_small(self.root/"revoked.der")
        with self.assertRaises(Stop):Validator(self.r,bad,self.cdp,self.verifier).validate(self.c,self.now)
        with self.assertRaises(Stop):self.issuer.command(["verify","-trusted","root.pem","-crl_check","-CRLfile","current.crl.pem","revoked.pem"])

    def test_wrong_EKU_signed_leaf_rejected(self):
        with self.assertRaises(Stop):Validator(self.r,read_small(self.root/"client.der"),self.cdp,self.verifier).validate(self.c,self.now)
        with self.assertRaises(Stop):self.issuer.command(["verify","-trusted","root.pem","-purpose","sslserver","client.pem"])

    def test_wrong_hostname_rejected_by_OpenSSL(self):
        with self.assertRaises(Stop):self.issuer.command(["verify","-trusted","root.pem","-verify_hostname","wrong.invalid","valid.pem"])

    def test_loopback_SAN_IP_and_localhost_verify(self):
        for flag,value in (("-verify_ip","127.0.0.1"),("-verify_ip","::1"),("-verify_hostname","localhost")):
            self.issuer.command(["verify","-trusted","root.pem",flag,value,"valid.pem"])

    def test_expired_signed_CRL_rejected(self):
        data=read_small(self.root/"expired.crl.der")
        with self.assertRaises(Stop):self.val.validate(data,self.now)

    def test_future_CRL_rejected_without_clock_change(self):
        with self.assertRaises(Stop):self.val.validate(self.c,crl(self.c)["start"]-1)

    def test_signature_corruption_rejected(self):
        bad=self.c[:-1]+bytes([self.c[-1]^1])
        with self.assertRaises(Stop):self.val.validate(bad,self.now)

    def test_signed_wrong_AKI_rejected(self):
        with self.assertRaises(Stop):self.val.validate(read_small(self.root/"wrong_aki.crl.der"),self.now)

    def test_other_issuer_CRL_rejected(self):
        with self.assertRaises(Stop):self.val.validate(self.foreign.public()[2],self.now)

    def test_wrong_public_root_rejected(self):
        with self.assertRaises(Stop):Validator(self.foreign.public()[0],self.l,self.cdp,self.verifier).validate(self.c,self.now)

    def test_wrong_CDP_rejected(self):
        with self.assertRaises(Stop):Validator(self.r,self.l,self.cdp+"/wrong",self.verifier).validate(self.c,self.now)

    def test_truncated_and_nonminimal_DER_rejected(self):
        for bad in (self.c[:-1],b"\x30\x80",b"\x30\x81\x01\0",self.c+b"\0"):
            with self.subTest(length=len(bad)),self.assertRaises((Stop,ValueError)):self.val.validate(bad,self.now)

    def test_creation_lock_consumed_on_interruption(self):
        obj=IssuerFixture(create_workspace(),MemoryFixtureVault())
        with self.assertRaises(Stop):obj.create(self.cdp,fail_after_key=True)
        key_hash=digest(read_small(obj.root/"ca.enc.pem"))
        with self.assertRaises(FileExistsError):obj.create(self.cdp)
        self.assertEqual(digest(read_small(obj.root/"ca.enc.pem")),key_hash)

    def test_recovered_fixture_vault_backup_restore(self):
        backup,target=create_workspace(),create_workspace()
        expected=self.issuer.backup(backup)
        result=self.issuer.restore(backup,target,expected,self.vault.recovered())
        self.assertTrue(result["public_identity_verified"])
        self.assertFalse(result["physical_second_location_verified"])

    def test_incorrect_recovered_credential_fails(self):
        with self.assertRaises(Stop):
            self.issuer.recover_identity(self.root/"ca.enc.pem",b"synthetic-wrong-credential")

    def test_custody_ciphertext_tamper_fails_manifest(self):
        backup,target=create_workspace(),create_workspace()
        expected=self.issuer.backup(backup)
        p=backup/"ca.enc.pem";p.write_bytes(read_small(p)+b"\n")
        with self.assertRaises(Stop):self.issuer.restore(backup,target,expected,self.vault.recovered())

    def test_custody_manifest_untrusted_hash_fails(self):
        backup,target=create_workspace(),create_workspace()
        self.issuer.backup(backup)
        with self.assertRaises(Stop):self.issuer.restore(backup,target,"0"*64,self.vault.recovered())

    def test_insecure_key_permissions_fail(self):
        backup,target=create_workspace(),create_workspace()
        expected=self.issuer.backup(backup)
        self.issuer.restore(backup,target,expected,self.vault.recovered())
        os.chmod(target/"ca.enc.pem",0o644)
        with self.assertRaises(Stop):self.issuer.recover_identity(target/"ca.enc.pem",self.vault.recovered())

    def test_fixture_path_and_symlink_fences(self):
        with self.assertRaises(Stop):owned(HERE)
        root=create_workspace();(root/"link").symlink_to(self.root/"root.pem")
        with self.assertRaises(Stop):child(root,"link")
        with self.assertRaises(Stop):child(root,"../root.pem")

    def test_publish_readback_and_monotonicity(self):
        store=Publication(create_workspace(),self.val)
        self.assertEqual(store.publish(self.c)["number"],0x2000)
        self.assertEqual(store.read(),self.c)
        with self.assertRaises(Stop):store.publish(self.c)
        store.publish(read_small(self.root/"successor.crl.der"))
        with self.assertRaises(Stop):store.publish(self.c)

    def test_publication_fault_before_water_does_not_serve_new_DER(self):
        store=Publication(create_workspace(),self.val);store.publish(self.c)
        with self.assertRaises(Stop):store.publish(read_small(self.root/"successor.crl.der"),fault="after_bundle")
        self.assertEqual(store.read(),self.c)

    def test_publication_gap_after_water_is_fail_closed(self):
        store=Publication(create_workspace(),self.val);store.publish(self.c)
        with self.assertRaises(Stop):store.publish(read_small(self.root/"successor.crl.der"),fault="after_water")
        with self.assertRaises(Stop):store.read()

    def test_complete_bundle_survives_fault_after_pointer(self):
        store=Publication(create_workspace(),self.val);store.publish(self.c)
        data=read_small(self.root/"successor.crl.der")
        with self.assertRaises(Stop):store.publish(data,fault="after_pointer")
        self.assertEqual(store.read(),data)

    def test_corrupt_pointer_and_bundle_cannot_serve(self):
        store=Publication(create_workspace(),self.val);store.publish(self.c)
        ptr=store.root/"current.json";p=json.loads(read_small(ptr))
        bundle=store.root/p["bundle"];bundle.write_bytes(read_small(bundle)+b" ")
        with self.assertRaises(Stop):store.read()

    def test_publisher_outage_does_not_fabricate_fetch_success(self):
        server=fixture_server(create_workspace())
        url=server.endpoint;server.server_close()
        with self.assertRaises((urllib.error.URLError,TimeoutError)):urllib.request.urlopen(url,timeout=2)

    def test_invalid_or_expired_publication_rejected(self):
        store=Publication(create_workspace(),self.val)
        with self.assertRaises(Stop):store.publish(read_small(self.root/"expired.crl.der"))
        with self.assertRaises(Stop):store.publish(self.c[:-1]+bytes([self.c[-1]^1]))

    def test_expiring_publication_margin(self):
        store=Publication(create_workspace(),self.val)
        with self.assertRaises(Stop):store.publish(self.c,now=crl(self.c)["end"]-3600)

    def test_publication_lock_contention_is_stop(self):
        root=create_workspace();store=Publication(root,self.val)
        with publication_lock(root):
            with self.assertRaises(Stop):store.publish(self.c)

    def test_HTTP_exact_path_get_head_and_reject_mutation(self):
        store=Publication(create_workspace(),self.val);store.publish(self.c)
        server=fixture_server(store.root,store)
        thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
        try:
            with urllib.request.urlopen(server.endpoint,timeout=3) as response:self.assertEqual(response.read(),self.c)
            with urllib.request.urlopen(urllib.request.Request(server.endpoint,method="HEAD"),timeout=3) as response:self.assertEqual(response.read(),b"")
            for url,method,status in ((server.endpoint+"?x","GET",404),(server.endpoint,"POST",405)):
                with self.assertRaises(urllib.error.HTTPError) as caught:urllib.request.urlopen(urllib.request.Request(url,method=method),timeout=3)
                self.assertEqual(caught.exception.code,status)
        finally:
            server.shutdown();server.server_close();thread.join(timeout=3)

    def test_HTTP_gap_is_503(self):
        store=Publication(create_workspace(),self.val);store.publish(self.c)
        with self.assertRaises(Stop):store.publish(read_small(self.root/"successor.crl.der"),fault="after_water")
        server=fixture_server(store.root,store);thread=threading.Thread(target=server.serve_forever,daemon=True);thread.start()
        try:
            with self.assertRaises(urllib.error.HTTPError) as caught:urllib.request.urlopen(server.endpoint,timeout=3)
            self.assertEqual(caught.exception.code,503)
        finally:
            server.shutdown();server.server_close();thread.join(timeout=3)

    def test_cached_historical_DER_does_not_prove_live_publisher(self):
        store=Publication(create_workspace(),self.val);store.publish(self.c)
        cached=store.read()
        with self.assertRaises(Stop):store.publish(read_small(self.root/"successor.crl.der"),fault="after_water")
        self.assertEqual(self.val.validate(cached,self.now)["number"],0x2000)
        with self.assertRaises(Stop):store.read()

    def test_live_approval_and_transition_routes_are_disabled(self):
        for fn in (approve_real,real_transition):
            with self.assertRaises(Stop):fn()

    def test_approval_scope_identity_and_effects(self):
        ident=fixture_approval("install_current_user_root")
        self.assertIn("Other applications",screen("install_current_user_root",ident))
        created=fixture_approval("create_issuer")
        self.assertIn("UNKNOWN",screen("create_issuer",created))
        created["root_sha256"]="0"*64
        with self.assertRaises(Stop):screen("create_issuer",created)
        ident["store"]="LocalMachine/Root"
        with self.assertRaises(Stop):screen("install_current_user_root",ident)
        ident=fixture_approval("retire_old_root")
        self.assertIn("BOTH DER hash",screen("retire_old_root",ident))
        ident["root_sha256"]="unknown"
        with self.assertRaises(Stop):screen("retire_old_root",ident)

    def test_actual_PKCS8_KDF_cipher_and_iteration_profile(self):
        data=read_small(self.root/"ca.enc.pem")
        der=base64.b64decode(b"".join(x for x in data.splitlines() if not x.startswith(b"-----")))
        outer=seq(der);alg=seq(outer[0][2])
        self.assertEqual(oid(alg[0]),"2a864886f70d01050d")
        params=seq(alg[1][2]);kdf=seq(params[0][2]);cipher=seq(params[1][2])
        self.assertEqual(oid(kdf[0]),"2a864886f70d01050c")
        values=seq(kdf[1][2])
        self.assertEqual(integer(values[1]),600000)
        self.assertEqual(oid(seq(values[-1][2])[0]),"2a864886f70d0209")
        self.assertEqual(oid(cipher[0]),"60864801650304012a")
        self.assertEqual(len(cipher[1][1]),16)

    def test_synthetic_context_source_expiry_and_scope(self):
        root=create_workspace();ctx=prepare(root)
        self.assertTrue(verify(ctx,root))
        for key,value in (("operation","create_real_CA"),("authority_SHA256","0"*64),
                          ("trust_authority",True),("implementation_accepted",True)):
            changed=copy.deepcopy(ctx);changed[key]=value
            with self.assertRaises(Stop):verify(changed,root)
        changed=copy.deepcopy(ctx);changed["source_SHA256"]["der.py"]="0"*64
        with self.assertRaises(Stop):verify(changed,root)
        old=prepare(root,datetime.datetime(2000,1,1,tzinfo=datetime.timezone.utc))
        with self.assertRaises(Stop):verify(old,root)
        bind_once(root)
        with self.assertRaises(FileExistsError):bind_once(root)

    def test_signed_CRL_lifecycle_uses_independent_ledger_and_dynamic_dates(self):
        counter=int((self.root/"crlnumber").read_text().strip(),16)
        result=renewal_plan(self.val,self.c,counter,self.now)
        self.assertEqual(result["renew_by"],crl(self.c)["end"]-72*3600)
        self.assertFalse(result["signing_enabled"])
        with self.assertRaises(Stop):renewal_plan(self.val,self.c,0x1002,self.now)
        with self.assertRaises(Stop):renewal_plan(self.val,self.c,counter,self.now,operation_hours=1000)

    def test_CA_transaction_and_backup_lock_contention_stop(self):
        with publication_lock(self.root):
            with self.assertRaises(Stop):self.issuer.backup(create_workspace())
            with self.assertRaises(Stop):self.issuer.command(["ca","-config","issuer.cnf","-status","1"])

if __name__=="__main__":
    result=unittest.TextTestRunner(verbosity=2).run(unittest.defaultTestLoader.loadTestsFromTestCase(PKITests))
    receipt={"scope":"actual disposable cryptographic fixtures, not real SFD PKI/TLS/adoption",
        "tests_run":result.testsRun,"failures":len(result.failures),"errors":len(result.errors),"skips":len(result.skipped),
        "test_names":unittest.defaultTestLoader.getTestCaseNames(PKITests),
        "synthetic_fixture_root":str(getattr(PKITests,"root","setup-failed")),
        "real_trust_changes":0,"real_issuer_operations":0,"Docker_operations":0}
    if result.wasSuccessful():
        context_bytes=(PKITests.root/"source-context.json").read_bytes()
        receipt["executed_context"]=json.loads(context_bytes)
        receipt["executed_context_sha256"]=digest(context_bytes)
    (HERE/"linux-results.json").write_text(json.dumps(receipt,indent=2)+"\n")
    raise SystemExit(0 if result.wasSuccessful() else 1)
