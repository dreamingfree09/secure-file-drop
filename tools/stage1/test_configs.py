import copy,json,pathlib,sys,unittest
HERE=pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
import acceptance
class Contracts(unittest.TestCase):
 def test_seven_client_members(self):
  m=json.loads((HERE/'client-members.json').read_text());self.assertEqual(len(m),7);self.assertEqual(len({x['path'] for x in m}),7)
  for x in m:self.assertEqual(set(x),{'path','bytes','SHA256'});self.assertEqual(len(x['SHA256']),64)
 def test_exact_runtime_pins(self):
  r=json.loads((HERE/'runtime-lock.json').read_text());self.assertEqual(r['versions']['traefik'],'3.7.17');self.assertEqual(r['versions']['rustfs'],'1.0.4')
  for v in r['images'].values():self.assertRegex(v,r'@sha256:[0-9a-f]{64}$')
 def test_no_silent_acceptance(self):
  r=acceptance.aggregate([]);self.assertFalse(r['accepted']);self.assertFalse(r['authorises_deployment']);self.assertTrue(all(c['status']=='NOT RUN' for c in r['checks']))
 def test_skips_and_unbound_pass_rejected(self):
  key=json.loads((HERE/'acceptance.json').read_text())['checks'][0]['id']
  for p in [{'id':key,'status':'SKIP'},{'id':key,'status':'PASS'}]:
   p['schema']='SFD_STAGE1_CHECK_RECEIPT_V1'
   with self.assertRaises(ValueError):acceptance.aggregate([p])
 def test_owner_visual_cannot_be_automated(self):
  p={'schema':'SFD_STAGE1_CHECK_RECEIPT_V1','id':'Explorer-QuickSend','status':'PASS','evidence_sha256':'a'*64,'operator':'Codex'}
  with self.assertRaises(ValueError):acceptance.aggregate([p])
 def test_source_keeps_trust_and_host_changes_separate(self):
  src=(HERE/'guest-windows.ps1').read_text();self.assertNotIn('Import-Certificate',src);self.assertNotIn('ServerCertificateCustomValidationCallback',src)
  probe=(HERE/'TlsProbe.cs').read_text();self.assertIn('.NETFramework,Version=v4.8',probe);self.assertIn('CheckCertificateRevocationList=true',probe)
 def test_offline_image_contract(self):
  c=(HERE/'compose.yml').read_text();self.assertEqual(c.count('pull_policy: never'),4)
 def test_no_media_or_security_acceptance_fabricated(self):
  b=json.loads((HERE/'bundle-example.json').read_text());self.assertFalse(b['complete']);self.assertFalse(b['runtime_acceptance_verified'])
  c=(HERE/'ci-runtime.py').read_text();self.assertIn("'full_security_acceptance':False",c)
 def test_backup_contract(self):
  c=(HERE/'compose.yml').read_text();self.assertIn('SFD_BACKUP_INTERVAL: 24h',c);self.assertIn('SFD_BACKUP_DIR: /var/backups/sfd',c);self.assertNotIn('INTERVAL_HOURS',c);self.assertIn('SFD_BACKUP_RETENTION_DAYS: "30"',c)
if __name__=='__main__':unittest.main()
