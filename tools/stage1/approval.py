"""Identity-separated V2 approval validation. No account creation or network IO.
Test policies are explicit in-memory fixtures; CLI always uses committed policy.
"""
import datetime as dt,hashlib,re
import guard
COMMON={'schema','role','signer_id','decision','work_id','repository','repository_id','pull_request_number','pull_request_id','source_sha','manifest_sha256','bundle_sha256','host_identity_sha256','effects','media_terms','nonce','issued_utc','expires_utc'}

def authority(policy):
 a=policy['approval_authority'];o=a['owner'];r=a['reviewer']
 if a.get('enrollment_accepted') is not True:raise guard.Stop('independent-enrollment-unaccepted')
 if any(type(x.get('id')) is not int or x['id']<=0 or not isinstance(x.get('login'),str) or not x['login'] for x in [o,r]):raise guard.Stop('independent-identity-unconfigured')
 if r['id'] in [o['id'],a['producer_id']] or r['login'].lower()==o['login'].lower():raise guard.Stop('independent-self-approval')
 return a

def packet(c,role,context,now,policy):
 a=authority(policy);identity=a['reviewer' if role=='Aether' else 'owner']
 if type(c) is not dict or type(c.get('id')) is not int or c['id']<=0:raise guard.Stop('comment-identity')
 if c.get('author_id')!=identity['id'] or c.get('login')!=identity['login'] or c.get('user_type')!='User':raise guard.Stop('approval-signer')
 if c.get('via_app') is not False or c.get('association') not in ['OWNER','MEMBER','COLLABORATOR']:raise guard.Stop('approval-auth-channel')
 if c.get('repository_id')!=a['repository_id'] or c.get('pull_request_id')!=a['pull_request_id'] or c.get('pull_request_number')!=a['pull_request_number']:raise guard.Stop('approval-pr-identity')
 created,updated=guard.iso(c['created_utc']),guard.iso(c['updated_utc'])
 if created!=updated or created>now:raise guard.Stop('approval-edited-or-future')
 body=c.get('body');match=re.fullmatch(r'SFD-STAGE1-ACCEPTANCE-V2\n(\{.*\})\n?',body or '',re.S)
 if not match:raise guard.Stop('approval-v2-marker')
 p=guard.strict_json(match[1]);keys=COMMON|({'review_comment_id','review_body_sha256'} if role=='Owner' else set())
 if type(p) is not dict or set(p)!=keys or p['schema']!='SFD_STAGE1_ACCEPTANCE_V2' or p['role']!=role or p['signer_id']!=identity['id'] or p['decision']!='GO':raise guard.Stop('approval-v2-fields')
 expected=dict(context,work_id=policy['work_id'],repository=policy['repository'],repository_id=a['repository_id'],pull_request_number=a['pull_request_number'],pull_request_id=a['pull_request_id'],effects=policy['effects'])
 for key,value in expected.items():
  if p.get(key)!=value or type(p.get(key)) is not type(value):raise guard.Stop('approval-context-'+key)
 if not re.fullmatch('[a-f0-9]{40}',p['source_sha']) or any(not isinstance(p[k],str) or not re.fullmatch('[a-f0-9]{64}',p[k]) for k in ['manifest_sha256','bundle_sha256','host_identity_sha256']):raise guard.Stop('approval-hash')
 if not isinstance(p['nonce'],str) or not re.fullmatch('[a-f0-9]{32}',p['nonce']) or p['media_terms'] is not True:raise guard.Stop('approval-nonce-or-terms')
 issued,expires=guard.iso(p['issued_utc']),guard.iso(p['expires_utc'])
 if not issued<=created<=now<expires or created-issued>dt.timedelta(minutes=5) or not dt.timedelta(0)<expires-issued<=dt.timedelta(hours=policy['max_lease_hours']):raise guard.Stop('approval-window')
 return p,hashlib.sha256(body.encode('utf-8')).hexdigest()

def validate_pair(review,owner,context,now,policy=None):
 policy=guard.POLICY if policy is None else policy;authority(policy)
 if review.get('id')==owner.get('id') or review.get('author_id')==owner.get('author_id'):raise guard.Stop('approval-distinct-signers')
 r,rhash=packet(review,'Aether',context,now,policy);o,ohash=packet(owner,'Owner',context,now,policy)
 if o['review_comment_id']!=review['id'] or type(o['review_comment_id']) is not int or o['review_body_sha256']!=rhash:raise guard.Stop('approval-review-reference')
 if o['nonce']!=r['nonce'] or guard.iso(o['issued_utc'])<guard.iso(r['issued_utc']) or guard.iso(o['expires_utc'])>guard.iso(r['expires_utc']):raise guard.Stop('approval-pair-window')
 return {'schema':'SFD_VERIFIED_APPROVAL_PAIR_V2','expires_utc':o['expires_utc'],'nonce':o['nonce'],'review':{'id':review['id'],'author_id':review['author_id'],'body_sha256':rhash},'owner':{'id':owner['id'],'author_id':owner['author_id'],'body_sha256':ohash}}
