import importlib.util,json,unittest,urllib.parse
from pathlib import Path
spec=importlib.util.spec_from_file_location('staged',Path(__file__).with_name('staged_account.py'));s=importlib.util.module_from_spec(spec);spec.loader.exec_module(s)

class StagedTests(unittest.TestCase):
 def test_generated_password_strength(self):
  vals={s.password() for _ in range(128)}
  self.assertEqual(len(vals),128)
  for p in vals:self.assertGreaterEqual(len(p),40);self.assertLessEqual(len(p.encode()),72);self.assertTrue(p.startswith('Sfd9!'))
 def test_exact_verify_origin(self):
  p='https://localhost:8443/verify?token='+'a'*64
  self.assertEqual(s.verify_path(p),p[len('https://localhost:8443'):])
 def test_reject_untrusted_verification_targets(self):
  token='a'*64
  for p in ['http://localhost:8443/verify?token='+token,'https://evil.invalid:8443/verify?token='+token,
   'https://localhost:8443/login?token='+token,'https://localhost:8443/verify?token='+token+'&extra=1',
   'https://localhost:8443/verify?token='+token+'&token='+token,'https://localhost:8443/verify?token='+token+'#fragment',
   'https://owner@localhost:8443/verify?token='+token,'https://localhost:8443/verify?token=short']:
   with self.subTest(p=p),self.assertRaises((RuntimeError,ValueError)):s.verify_path(p)
 def test_message_delivered_to_exact_alias(self):
  raw=('To: controlled+fixture@example.invalid\r\nContent-Type: text/plain\r\n\r\nhttps://localhost:8443/verify?token='+'b'*64).encode()
  self.assertTrue(s.mail_path(raw,'controlled+fixture@example.invalid').startswith('/verify?token='))
  with self.assertRaises(RuntimeError):s.mail_path(raw,'unrelated@example.invalid')
 def test_multiple_verification_tokens_rejected(self):
  raw=('To: controlled@example.invalid\r\n\r\nhttps://localhost:8443/verify?token='+'b'*64+'\nhttps://localhost:8443/verify?token='+'c'*64).encode()
  with self.assertRaises(RuntimeError):s.mail_path(raw,'controlled@example.invalid')
 def test_account_name_bound(self):
  self.assertTrue(s.NAME.fullmatch('sfd_acceptance_20261026'))
  self.assertTrue(s.NAME.fullmatch('sfd_acceptance_20261026_0123abcd'))
  for name in ['admin','existing-user','sfd_acceptance_20261026_admin']:
   self.assertFalse(s.NAME.fullmatch(name))
 def test_review_is_required(self):
  for cfg in [{},{'master':s.MASTER,'candidate':s.IMAGE,'execution_authorised':False}]:
   with self.assertRaises((RuntimeError,KeyError)):s.decision(cfg)
 def test_conditional_scope_decision_not_new_helper_acceptance(self):
  cfg={'master':s.MASTER,'candidate':s.IMAGE,'execution_authorised':True,'independent_review':'https://github.com/dreamingfree09/secure-file-drop/pull/35#issuecomment-6413982287'}
  with self.assertRaises(RuntimeError):s.decision(cfg)
if __name__=='__main__':unittest.main()
