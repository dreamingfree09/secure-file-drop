"""One-use local controller policy. No credentials enter this module.
Live use requires a new exact-revision Aether controller acceptance. PR37
acceptance authorises preparation only; the two decisions cannot substitute.
"""
import collections,datetime,hashlib,json,os,re,stat,time,secrets
from pathlib import Path

MASTER='61195434bee5f9221a6f55469c719cb360504ea9'
PROTOCOL_MASTER='ae45cb816f9ec05e334e4620a4b428cebe631901'
OPERATOR_HEAD='47be19fa5ed0ffddd9ffea71fec84719f2a2b5cc'
OPERATOR_TREE='0424c1d4cd375c21801e33775f28f73ab06b151b'
WORK='SFD-OWNER-TEST-ACCOUNT-20261026'
IMAGE='sha256:084afb9ff9ecd6244f24e795e86f35cd8d328130915bc44aa0d96ebfe0e356f2'
RECOVERY='sha256:4c60abc87c6d5b2b9d56f95b3d3255f1b5900695de7554ab2ca4cfe5dde47c07'
PGIMAGE='sha256:d4c3314b2dd74ff23cd6cd07e4d7e737cb6ef791c22a7cbd744cfbfb4815df72'
PINS={
 'legacy_context':'6c65e4a2c990a0740585b9057589841aacba39dfda0985a07419ee51d99a46b9',
 'scope':'0ffabb47d13d9aaa46508b7c88192307be007d18628b20dc9674a60cc58c4573',
 'prior_plan':'750588f2c2b53fd3b06e304928ea62fc1811f734c2c326a94d247948649e6db1',
 'current_manifest':'3283c6ec9c2a02087c27174305d0411bcbbc690b2377de2ae38f9b5866d55eba',
 'baseline':'3b6760532ec3df5149998184563e0e59aa374216dba28829cb6992a6f9fe3ab1',
 'restore_utility':'252ed319521785c533f9dbb80f7e1bb4c8915356f0e96fe93be86daa824f0240',
 'restore_source':'f31536730ffbf1c2a3d1db6b3e58e61c9a78819047b6429a10317c6d3b6a2194',
 'image_overlay':'4254071672538fe22aa63cac1e725b4ff81513f04fd3c1478c0c0713dd33b7cf',
 'retention_overlay':'e083dd03d918127b8dda6d73c9cc8171777b9eb2d2031112c48b5b56d4e70dfd',
 'operator_binary':'43eec7c68d480e4ebc3dfd7063fe37ecd3f0f6ba0e6917003cc5e3f5604944bc',
 'operator_driver':'127f454646e59d01d05a8cbdf116918f6129512585e3183059de034a064c9adc',
 'native_form':'91bec8d81a9b3918c0b0cfda09d0182cc0b7b9329541a680c09c5b609c79300d',
 'transport':'11a12387243855c29064e7cda9d029161c495cc73e05d435b4dbfa1df829726a',
 'F196':'f196bab87a426d6a5d287f16c199259edb7993e15c4e35d2886d4a196a6fa0ca',
 'framework_config':'4d83c0772f4b593d6e16657fb5be7e4478321cb9782ba744c93b90915fb7175b',
 'live_acceptance':'6fc1bf1e95ebe929440f038ee6a8538adcdb44ea0d6a02c174ccb751ea9f2456',
 'backend':'f4dc65ff58a23f1026f19b9d1eb542e70ee35a2a8d9e3c3862d044263b58f4a1',
 'native_hash':'3cd81bf959aca7364d646a99f0cc8498b43729cc896ea80a0a6cd861591015ab',
 'CRL':'69bbe78994b7e6abe9849e65fd8a56fe59dbe0803e6db498f3e8c82ad70f6f1e',
}
REVIEWS={
 6427391643:'d814c0cdc8e43ec253d3ecd88ba1fe2386c3f1cf0e3f48105ed28e7b55c34181',
 6427498111:'f25515af561bf8cf89e0128197b3f0e26f492d5a42278bc7b4f178f980a107c8',
 6427401030:'ce774c1b3efe9fa08d766cb4f7aa352dff3c57c1c5db9c4b07b02a37d1f9d7aa',
}
RENEW_BY=datetime.datetime(2026,10,28,18,10,29,tzinfo=datetime.timezone.utc).timestamp()
MIN_MARGIN=5400
class GateFailure(RuntimeError):
    """Public messages contain a fixed stage only, never exception arguments."""
def need(value,stage):
    if not value:raise GateFailure(stage)
def digest(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def private_file(p,mode=None):
    p=Path(p);s=p.lstat()
    need(stat.S_ISREG(s.st_mode) and s.st_uid==os.getuid() and not(stat.S_IMODE(s.st_mode)&0o077),'private-file')
    if mode is not None:need(stat.S_IMODE(s.st_mode)==mode,'private-mode')
    return p
def private_json(p):return json.loads(private_file(p).read_text())
def new_json(p,value):
    p=Path(p);stage=p.parent/('.controller-stage-'+secrets.token_hex(12))
    fd=os.open(stage,os.O_CREAT|os.O_EXCL|os.O_WRONLY|os.O_NOFOLLOW,0o600)
    try:
        with os.fdopen(fd,'w') as f:
            json.dump(value,f,sort_keys=True);f.write('\n');f.flush();os.fsync(f.fileno())
        os.link(stage,p,follow_symlinks=False)
        d=os.open(p.parent,os.O_RDONLY|os.O_DIRECTORY)
        try:os.fsync(d)
        finally:os.close(d)
    finally:
        if stage.exists():stage.unlink()
def canonical(value):return hashlib.sha256(json.dumps(value,sort_keys=True,separators=(',',':')).encode()).hexdigest()
def validate_review(comment,cfg):
    need(comment.get('user',{}).get('login')=='dreamingfree09','controller-review-author')
    need(comment.get('issue_url','').endswith('/issues/'+str(cfg['controller_pr'])),'controller-review-thread')
    body=comment['body'];need(hashlib.sha256(body.encode()).hexdigest()==cfg['controller_review_body_sha256'],'controller-review-body')
    blocks=re.findall(r'```json\s*([\s\S]*?)\s*```',body)
    decisions=[]
    for raw in blocks:
        try:v=json.loads(raw)
        except ValueError:continue
        if isinstance(v,dict) and v.get('kind')=='SFD_CONTROLLER_ACCEPTANCE_V1':decisions.append(v)
    need(len(decisions)==1,'controller-review-unique')
    d=decisions[0]
    need(d.get('accepted') is True and d.get('revision')==cfg['controller_revision'] and
         d.get('bundle_sha256')==cfg['bundle_sha256'] and d.get('probe_sha256')==cfg['probe_sha256'] and d.get('master')==MASTER and
         d.get('scope_plan_sha256')==PINS['scope'] and d.get('operator_review_comment')==6427391643,
         'controller-exact-acceptance')
    return d
def validate_ready(r,now):
    need(r.get('master')==MASTER and r.get('candidate')==IMAGE and
         r.get('administrator_password_match') is True and r.get('Owner_confirmed_IDLE') is True and
         r.get('registered_password_requested') is False,'admin-only-readiness')
    need(isinstance(r.get('pid'),int) and r['pid']>1 and isinstance(r.get('Windows_PID'),int) and r['Windows_PID']>1,'ready-process')
    need(re.fullmatch('[a-f0-9]{48}',r.get('nonce','')) is not None,'ready-nonce')
    exp=datetime.datetime.fromisoformat(r['expires_utc']).timestamp()
    need(300<exp-now<=1201,'ready-expiry')
def original_rows_preserved(before,after,additions=False):
    need(set(before)<=set(after),'original-tables')
    for t,rows in before.items():
        if t=='schema_migrations':continue
        old=collections.Counter(rows);new=collections.Counter(after[t])
        need(not(old-new),'original-row-preservation')
        if not additions:need(old==new,'migration-only-rows')
def snapshot_preserved(before,after,new_file=None):
    need(before.get('policy_sha256')==after.get('policy_sha256'),'storage-policy-preserved')
    old={v['id']:v for v in before['objects']};new={v['id']:v for v in after['objects']}
    need(all(new.get(k)==v for k,v in old.items()),'original-object-preservation')
    if new_file is None:need(old==new,'unexpected-object-change')
    else:
        extra=[v for k,v in new.items() if k not in old]
        need(len(extra)==1 and extra[0]['bytes']==512 and extra[0]['sha256']==new_file['sha256'],'new-object-integrity')
class Controller:
    """Orchestrator: tests inject a fixture Runtime; CLI never permits this choice."""
    def __init__(self,runtime):self.r=runtime;self.stage='authority';self.stopped=False;self.restore_count=0;self.owns_session=False
    def execute(self):
        r=self.r
        try:
            r.authority();r.acquire_once();self.owns_session=True;r.event('controller-started')
            self.stage='preflight';pre=r.preflight();need(len(r.targets)==3,'three-archive-count');r.prepare_input(pre)
            self.stage='owner';ready=r.wait_owner();validate_ready(ready,r.now());r.check_owner(ready)
            # Never assume a prior receipt stays valid while awaiting the Owner.
            self.stage='recheck';r.recheck(pre,[],ready,deep=True)
            self.stage='restore'
            for spec in r.targets:
                r.recheck(pre,r.restored,ready)
                r.restore_one(spec);self.restore_count+=1
                r.event('archive-published',index=self.restore_count,group_atomic=False)
            r.recheck(pre,r.restored,ready)
            self.stage='stop';r.stop_old();self.stopped=True
            self.stage='adopt';r.adopt();r.verify_adopted(pre)
            self.stage='account';r.reserve_account_once();r.trigger_operator(ready)
            result=r.wait_result()
            need(result.get('success') is True and result.get('count')==27,'F196-live-result')
            self.stage='final-preservation';r.verify_final(pre,result)
            # The new broker holds ACCOUNT_ACTIVE/password until this verdict.
            self.stage='handoff';r.release_handoff(ready,result)
            r.event('complete',actual_master=MASTER,operator_reference_master=PROTOCOL_MASTER)
            return {'success':True,'actual_master':MASTER,'restored':self.restore_count,'Owner_visual_acceptance':False}
        except Exception as failure:
            # A denied authority/lease must not write into, abort or disrupt an
            # existing context owned by another running controller.
            if self.owns_session:
                stop_ok=None
                if self.stopped:
                    try:r.fail_closed_backend();stop_ok=True
                    except Exception:stop_ok=False
                try:r.abort_handoff(self.stage)
                except Exception:pass
                try:
                    r.event('stopped',stage=self.stage,reason=str(failure) if isinstance(failure,GateFailure) else 'bounded-internal-exception',restored_count=self.restore_count,
                            completed_subset_retained=True,fail_closed_stop_verified=stop_ok,
                            password_may_have_been_disclosed=bool(getattr(r,'handoff_released',False)))
                except Exception:pass
            raise GateFailure(self.stage) from None
        finally:r.finish()
