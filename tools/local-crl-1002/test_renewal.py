import datetime as dt
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("renew", Path(__file__).with_name("owner-renew-crl.py"))
renew = importlib.util.module_from_spec(spec)
spec.loader.exec_module(renew)

class RenewalGates(unittest.TestCase):
    def test_sha_known_vector(self):
        self.assertEqual(renew.digest(b"abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
    def test_regular_rejects_symlink(self):
        with tempfile.TemporaryDirectory() as temp:
            target=Path(temp)/"target";target.write_bytes(b"public")
            link=Path(temp)/"link";link.symlink_to(target)
            with self.assertRaisesRegex(renew.Stop,"SYMLINK"):renew.regular(link)
    def test_pin_mismatch(self):
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/"public";path.write_bytes(b"changed")
            with self.assertRaisesRegex(renew.Stop,"PIN_MISMATCH"):renew.read_bound(path,"0"*64)
    def test_private_permissions(self):
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/"private";path.write_bytes(b"synthetic");path.chmod(0o644)
            with self.assertRaisesRegex(renew.Stop,"PRIVATE_FILE_PERMISSIONS"):renew.regular(path,True)
    def test_key_stat_never_opens_bytes(self):
        with tempfile.TemporaryDirectory() as temp:
            path=Path(temp)/"key";path.write_bytes(b"NOT A KEY");path.chmod(0o600)
            with patch.object(Path,"read_bytes",side_effect=AssertionError("key read")):
                self.assertEqual(renew.key_metadata(path)["bytes"],9)
    def test_review_missing_refuses_before_sign(self):
        with patch.object(renew,"check_context",return_value={"helper_sha256":"0"*64}), patch.object(renew.subprocess,"run",side_effect=AssertionError("executed")):
            with self.assertRaisesRegex(renew.Stop,"SIGNING_REVIEW_REQUIRED"):
                renew.sign(Path("/synthetic/context"),"1"*64,None,None)
    def test_review_cannot_reuse_preparation_approval(self):
        body={"signing_authorized":True,"reviewer":"Aether","comment_id":6454489528,"comment_body_sha256":"a"*64}
        with patch.object(renew,"read_bound",return_value=json.dumps(body).encode()):
            with self.assertRaisesRegex(renew.Stop,"EXACT_REVIEW_BINDING_REQUIRED"):
                renew.verify_acceptance(Path("/synthetic"),"a"*64,"b"*64,"c"*64,dt.datetime.now(dt.timezone.utc))
    def test_review_wrong_identity(self):
        now=dt.datetime.now(dt.timezone.utc)
        body={"signing_authorized":True,"reviewer":"Aether","comment_id":6454489529,"comment_body_sha256":"a"*64,"context_sha256":"wrong","helper_sha256":"c"*64,"valid_until":(now+dt.timedelta(hours=1)).isoformat()}
        with patch.object(renew,"read_bound",return_value=json.dumps(body).encode()):
            with self.assertRaisesRegex(renew.Stop,"REVIEW_IDENTITY"):
                renew.verify_acceptance(Path("/synthetic"),"a"*64,"b"*64,"c"*64,now)
    def test_review_expired(self):
        now=dt.datetime.now(dt.timezone.utc)
        body={"signing_authorized":True,"reviewer":"Aether","comment_id":6454489529,"comment_body_sha256":"a"*64,"context_sha256":"b"*64,"helper_sha256":"c"*64,"valid_until":(now-dt.timedelta(seconds=1)).isoformat()}
        with patch.object(renew,"read_bound",return_value=json.dumps(body).encode()):
            with self.assertRaisesRegex(renew.Stop,"REVIEW_WINDOW"):
                renew.verify_acceptance(Path("/synthetic"),"a"*64,"b"*64,"c"*64,now)
    def test_expected_new_fields(self):
        now=dt.datetime(2026,10,28,10,tzinfo=dt.timezone.utc)
        text="issuer=CN = Secure File Drop Local Development CA\ncrlNumber=0x1002\nlastUpdate=Oct 28 10:00:00 2026 GMT\nnextUpdate=Nov  4 10:00:00 2026 GMT"
        self.assertIn("next_update",renew.crl_fields(text,"0x1002",now))
    def test_wrong_new_number(self):
        with self.assertRaisesRegex(renew.Stop,"CRL_NUMBER"):
            renew.crl_fields("crlNumber=0x1001","0x1002",dt.datetime.now(dt.timezone.utc))
    def test_wrong_new_lifetime(self):
        text="issuer=CN = Secure File Drop Local Development CA\ncrlNumber=0x1002\nlastUpdate=Oct 28 10:00:00 2026 GMT\nnextUpdate=Nov  5 10:00:00 2026 GMT"
        with self.assertRaisesRegex(renew.Stop,"CRL_LIFETIME"):
            renew.crl_fields(text,"0x1002",dt.datetime(2026,10,28,10,tzinfo=dt.timezone.utc))
    def test_public_command_failure_sanitized(self):
        class Result: returncode=1;stdout=b"";stderr=b"private diagnostic"
        with patch.object(renew.subprocess,"run",return_value=Result()):
            with self.assertRaisesRegex(renew.Stop,"PUBLIC_OPENSSL_CHECK_FAILED"):
                renew.run_public(["synthetic"])
    def test_no_publication_or_insecure_commands(self):
        text=Path(renew.__file__).read_text()
        self.assertNotIn("-passin",text)
        self.assertNotIn("os.replace(",text)
        self.assertNotIn("shutil.copy",text)
        self.assertNotIn("verify_none",text)


    def test_wrong_issuer(self):
        text="issuer=CN = Another CA\ncrlNumber=0x1002"
        with self.assertRaisesRegex(renew.Stop,"CRL_ISSUER"):
            renew.crl_fields(text,"0x1002",dt.datetime.now(dt.timezone.utc))
    def test_stale_new_crl(self):
        text="issuer=CN = Secure File Drop Local Development CA\ncrlNumber=0x1002\nlastUpdate=Oct 28 10:00:00 2026 GMT\nnextUpdate=Nov  4 10:00:00 2026 GMT"
        with self.assertRaisesRegex(renew.Stop,"CRL_START"):
            renew.crl_fields(text,"0x1002",dt.datetime(2026,10,28,11,tzinfo=dt.timezone.utc))
    def test_unapproved_review(self):
        with patch.object(renew,"read_bound",return_value=b'{"signing_authorized":false}'):
            with self.assertRaisesRegex(renew.Stop,"SIGNING_NOT_AUTHORIZED"):
                renew.verify_acceptance(Path("/synthetic"),"a"*64,"b"*64,"c"*64,dt.datetime.now(dt.timezone.utc))
    def test_one_use_collision_blocks_before_openssl(self):
        with tempfile.TemporaryDirectory() as temp:
            root=Path(temp);(root/"config").write_bytes(b"synthetic");(root/"started.json").write_bytes(b"existing")
            context={"helper_sha256":"a"*64,"pins":{"config":{"path":str(root/"config")}},"output_dir":str(root)}
            with patch.object(renew,"check_context",return_value=context),patch.object(renew,"verify_acceptance"),patch.object(renew.sys.stdin,"isatty",return_value=True),patch.object(renew.sys.stdout,"isatty",return_value=True),patch.object(renew.subprocess,"run",side_effect=AssertionError("unexpected subprocess")):
                with self.assertRaisesRegex(renew.Stop,"ONE_USE_OUTPUT_EXISTS"):
                    renew.sign(root/"context","b"*64,root/"acceptance","c"*64)
                self.assertEqual((root/"started.json").read_bytes(),b"existing")
    def test_non_tty_blocks_before_openssl(self):
        with patch.object(renew,"check_context",return_value={"helper_sha256":"a"*64}),patch.object(renew,"verify_acceptance"),patch.object(renew.sys.stdin,"isatty",return_value=False),patch.object(renew.subprocess,"run",side_effect=AssertionError("executed")):
            with self.assertRaisesRegex(renew.Stop,"LOCAL_TTY_REQUIRED"):
                renew.sign(Path("/synthetic/context"),"b"*64,Path("/synthetic/acceptance"),"c"*64)
    def test_review_naive_time_refused(self):
        body={"signing_authorized":True,"reviewer":"Aether","comment_id":6454489529,"comment_body_sha256":"a"*64,"context_sha256":"b"*64,"helper_sha256":"c"*64,"valid_until":"2026-10-28T12:00:00"}
        with patch.object(renew,"read_bound",return_value=json.dumps(body).encode()):
            with self.assertRaisesRegex(renew.Stop,"REVIEW_WINDOW"):
                renew.verify_acceptance(Path("/synthetic"),"a"*64,"b"*64,"c"*64,dt.datetime.now(dt.timezone.utc))

if __name__=="__main__":
    unittest.main(verbosity=2)
