import copy,datetime as dt,hashlib,json,unittest
import approval,guard
class IndependentApprovalTests(unittest.TestCase):
 def fixture(self):
  p=copy.deepcopy(guard.POLICY);a=p['approval_authority'];a.update(enrollment_accepted=True,producer_id=101,owner={'id':101,'login':'synthetic-owner'},reviewer={'id':202,'login':'synthetic-reviewer'})
  now=dt.datetime(2026,11,2,12,tzinfo=dt.timezone.utc);issued=now-dt.timedelta(minutes=1)
  context=dict(source_sha='a'*40,manifest_sha256='b'*64,bundle_sha256='c'*64,host_identity_sha256='d'*64)
  base=dict(context,schema='SFD_STAGE1_ACCEPTANCE_V2',decision='GO',work_id=p['work_id'],repository=p['repository'],repository_id=a['repository_id'],pull_request_number=a['pull_request_number'],pull_request_id=a['pull_request_id'],effects=p['effects'],media_terms=True,nonce='f'*32,issued_utc=issued.isoformat(),expires_utc=(now+dt.timedelta(hours=1)).isoformat())
  def comment(role,id,signer):
   packet=dict(base,role=role,signer_id=signer['id'])
   return dict(id=id,author_id=signer['id'],login=signer['login'],user_type='User',association='COLLABORATOR',via_app=False,repository_id=a['repository_id'],pull_request_id=a['pull_request_id'],pull_request_number=a['pull_request_number'],created_utc=issued.isoformat(),updated_utc=issued.isoformat(),body='SFD-STAGE1-ACCEPTANCE-V2\n'+json.dumps(packet))
  r=comment('Aether',11,a['reviewer']);o=comment('Owner',12,a['owner']);x=json.loads(o['body'].split('\n',1)[1]);x.update(review_comment_id=r['id'],review_body_sha256=hashlib.sha256(r['body'].encode()).hexdigest());o['body']='SFD-STAGE1-ACCEPTANCE-V2\n'+json.dumps(x)
  return p,now,context,r,o
 def test_real_identity_is_unenrolled(self):
  p,n,c,r,o=self.fixture()
  with self.assertRaises(guard.Stop):approval.validate_pair(r,o,c,n)
 def test_distinct_fixture_accounts_exact_context(self):
  p,n,c,r,o=self.fixture();v=approval.validate_pair(r,o,c,n,p);self.assertEqual(v['review']['author_id'],202);self.assertEqual(v['owner']['author_id'],101)
 def test_shared_identity_and_producer_review_denied(self):
  for identity in [101,303]:
   p,n,c,r,o=self.fixture();p['approval_authority']['reviewer']['id']=identity;p['approval_authority']['producer_id']=identity
   with self.subTest(identity=identity):
    with self.assertRaises(guard.Stop):approval.validate_pair(r,o,c,n,p)
 def test_role_channel_identity_and_edit_faults(self):
  for key,bad in [('id',12),('author_id',101),('login','synthetic-owner'),('user_type','Bot'),('association','CONTRIBUTOR'),('via_app',True),('repository_id',1),('pull_request_id',1),('pull_request_number',44),('updated_utc','2026-11-02T12:00:00+00:00')]:
   p,n,c,r,o=self.fixture();r[key]=bad
   with self.subTest(key=key):
    with self.assertRaises(guard.Stop):approval.validate_pair(r,o,c,n,p)
 def test_changed_packet_context_expiry_and_reference_denied(self):
  for key,bad in [('source_sha','e'*40),('manifest_sha256','e'*64),('bundle_sha256','e'*64),('host_identity_sha256','e'*64),('role','Aether'),('signer_id',202),('review_comment_id',99),('review_body_sha256','e'*64),('expires_utc','2026-11-02T11:59:00+00:00'),('effects',[]),('nonce','0'*32),('media_terms',False),('pull_request_id',1)]:
   p,n,c,r,o=self.fixture();body=json.loads(o['body'].split('\n',1)[1]);body[key]=bad;o['body']='SFD-STAGE1-ACCEPTANCE-V2\n'+json.dumps(body)
   with self.subTest(key=key):
    with self.assertRaises(guard.Stop):approval.validate_pair(r,o,c,n,p)
 def test_review_body_changed_after_owner_binding(self):
  p,n,c,r,o=self.fixture();r['body']+='\n'
  with self.assertRaises(guard.Stop):approval.validate_pair(r,o,c,n,p)
 def test_legacy_single_account_never_authorises(self):
  p,n,c,r,o=self.fixture()
  with self.assertRaises(guard.Stop):guard.validate_packet(r,'Aether',c['source_sha'],45,n)
if __name__=='__main__':unittest.main()
