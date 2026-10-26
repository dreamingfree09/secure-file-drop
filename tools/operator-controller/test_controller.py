"""Synthetic policy/process-boundary regressions; no live service or UI access."""
import contextlib,datetime,hashlib,importlib.util,json,os,stat,tempfile,threading,time,unittest
from pathlib import Path
from unittest.mock import patch
from policy import *
from broker import HandoffBuffer

class FixtureRuntime:
    def __init__(self,fail=None,phase=None,ledger=None):
        self.fail=fail;self.phase=phase;self.calls=[];self.targets=[{'name':str(n)} for n in range(3)]
        self.restored=[];self.handoff_released=False;self.used=False;self.ledger=ledger if ledger is not None else {}
        self.ready={'master':MASTER,'candidate':IMAGE,'administrator_password_match':True,'Owner_confirmed_IDLE':True,
                    'registered_password_requested':False,'pid':100,'Windows_PID':101,'nonce':'a'*48,
                    'expires_utc':datetime.datetime.fromtimestamp(time.time()+1200,datetime.timezone.utc).isoformat()}
    def step(self,name):
        self.calls.append(name)
        if name==self.fail:raise GateFailure('synthetic-failure')
    def now(self):return time.time()
    def authority(self):self.step('authority')
    def acquire_once(self):
        self.step('acquire')
        need(not self.used and not self.ledger.get('reserved'),'fixture-once');self.used=True
    def event(self,name,**fields):self.calls.append(('event',name,fields))
    def preflight(self):self.step('preflight');return {'synthetic':True}
    def prepare_input(self,pre):self.step('launch')
    def wait_owner(self):self.step('owner');return self.ready
    def check_owner(self,r):self.step('owner-proof')
    def recheck(self,pre,restored,ready,deep=False):
        self.step('recheck-'+str(len(restored)))
        if self.phase=='after-first' and len(restored)==1:raise GateFailure('new-scheduler-job')
    def restore_one(self,s):
        self.step('restore-'+s['name']);self.restored.append(s['name'])
        if self.phase=='after-second-publication' and len(self.restored)==2:raise GateFailure('durability-unconfirmed')
    def stop_old(self):self.step('stop-old')
    def adopt(self):self.step('adopt')
    def verify_adopted(self,pre):self.step('verify-adopted')
    def reserve_account_once(self):
        self.step('reserve-account');need(not self.ledger.get('reserved'),'one-account');self.ledger['reserved']=True
    def trigger_operator(self,r):self.step('trigger-account')
    def wait_result(self):
        self.step('result')
        return {'success':self.phase!='account-partial','count':27,'username':'sfd_acceptance_20261026'}
    def verify_final(self,pre,r):self.step('final-preservation')
    def release_handoff(self,ready,result):
        self.handoff_released=True;self.step('handoff')
    def fail_closed_backend(self):self.calls.append('stop-successor-only')
    def abort_handoff(self,stage):self.calls.append('abort-private-handoff')
    def finish(self):self.calls.append('finish')

class ControllerOrderingTests(unittest.TestCase):
    def test_success_sequence_and_exact_scope(self):
        f=FixtureRuntime();r=Controller(f).execute()
        self.assertEqual(r['actual_master'],MASTER);self.assertEqual(r['restored'],3);self.assertFalse(r['Owner_visual_acceptance'])
        events=[v for v in f.calls if isinstance(v,str)]
        for a,b in [('preflight','launch'),('owner-proof','restore-0'),('restore-2','stop-old'),('stop-old','adopt'),
                    ('verify-adopted','reserve-account'),('reserve-account','trigger-account'),('final-preservation','handoff')]:
            self.assertLess(events.index(a),events.index(b))
        self.assertEqual([v for v in events if v.startswith('restore-')],['restore-0','restore-1','restore-2'])
        self.assertNotIn('stop-successor-only',events)
    def test_every_gate_fails_closed(self):
        for stage in ['authority','acquire','preflight','launch','owner','owner-proof','restore-0','stop-old','adopt','verify-adopted','reserve-account','trigger-account','result','final-preservation','handoff']:
            with self.subTest(stage=stage):
                f=FixtureRuntime(fail=stage)
                with self.assertRaises(GateFailure):Controller(f).execute()
                if stage in ['authority','acquire']:self.assertNotIn('abort-private-handoff',f.calls)
                else:self.assertIn('abort-private-handoff',f.calls)
                self.assertEqual(f.calls[-1],'finish')
                if stage in ['adopt','verify-adopted','reserve-account','trigger-account','result','final-preservation','handoff']:
                    self.assertIn('stop-successor-only',f.calls)
                else:self.assertNotIn('stop-successor-only',f.calls)
                if stage!='handoff':self.assertFalse(f.handoff_released)
    def test_partial_group_publication_is_never_undone(self):
        for mode,count in [('after-first',1),('after-second-publication',2)]:
            f=FixtureRuntime(phase=mode)
            with self.assertRaises(GateFailure):Controller(f).execute()
            self.assertEqual(len(f.restored),count);self.assertNotIn('stop-old',f.calls);self.assertNotIn('handoff',f.calls)
    def test_four_archive_expansion_rejected(self):
        f=FixtureRuntime();f.targets.append({'name':'fourth'})
        with self.assertRaises(GateFailure):Controller(f).execute()
        self.assertNotIn('launch',f.calls)
    def test_context_is_one_use_even_if_no_owner_input(self):
        f=FixtureRuntime(fail='owner')
        with self.assertRaises(GateFailure):Controller(f).execute()
        f.fail=None;old=len(f.calls)
        with self.assertRaises(GateFailure):Controller(f).execute()
        self.assertNotIn('launch',f.calls[old:]);self.assertNotIn('abort-private-handoff',f.calls[old:])
    def test_partial_account_blocks_any_second_context(self):
        ledger={};first=FixtureRuntime(phase='account-partial',ledger=ledger)
        with self.assertRaises(GateFailure):Controller(first).execute()
        self.assertTrue(ledger['reserved']);self.assertFalse(first.handoff_released)
        second=FixtureRuntime(ledger=ledger)
        with self.assertRaises(GateFailure):Controller(second).execute()
        self.assertNotIn('launch',second.calls)
    def test_invalid_owner_states_never_restore(self):
        mutations=[{'Owner_confirmed_IDLE':False},{'administrator_password_match':False},
                   {'registered_password_requested':True},{'master':PROTOCOL_MASTER},{'nonce':'short'},
                   {'Windows_PID':0},{'expires_utc':datetime.datetime.now(datetime.timezone.utc).isoformat()}]
        for m in mutations:
            with self.subTest(fields=list(m)):
                f=FixtureRuntime();f.ready.update(m)
                with self.assertRaises(GateFailure):Controller(f).execute()
                self.assertFalse(f.restored);self.assertNotIn('stop-old',f.calls)
    def test_handoff_failure_records_possible_disclosure(self):
        f=FixtureRuntime(fail='handoff')
        with self.assertRaises(GateFailure):Controller(f).execute()
        stopped=[v for v in f.calls if isinstance(v,tuple) and v[1]=='stopped'][0]
        self.assertTrue(stopped[2]['password_may_have_been_disclosed'])

class PreservationTests(unittest.TestCase):
    def test_original_database_rows_preserved_with_additions(self):
        before={'users':[b'original'], 'files':[b'one',b'two'],'schema_migrations':[b'11']}
        after={'users':[b'original',b'new'],'files':[b'one',b'two',b'test'],'schema_migrations':[b'12'],'auth_sessions':[]}
        original_rows_preserved(before,after,True)
        with self.assertRaises(GateFailure):original_rows_preserved(before,after,False)
        for change in ['delete','alter','duplicate-loss']:
            bad={k:list(v) for k,v in after.items()}
            bad['files']=[b'one',b'changed'] if change=='alter' else [b'one']
            with self.assertRaises(GateFailure):original_rows_preserved(before,bad,True)
    def test_original_object_data_and_policy_preserved(self):
        old={'policy_sha256':'private','objects':[{'id':'old','bytes':3,'sha256':'abc'}]}
        new={'policy_sha256':'private','objects':old['objects']+[{'id':'test','bytes':512,'sha256':'new'}]}
        snapshot_preserved(old,new,{'sha256':'new'})
        for case in ['deleted','changed','public','two-new','wrong-size']:
            bad=json.loads(json.dumps(new))
            if case=='deleted':bad['objects'].pop(0)
            elif case=='changed':bad['objects'][0]['sha256']='changed'
            elif case=='public':bad['policy_sha256']='changed'
            elif case=='two-new':bad['objects'].append({'id':'extra'})
            else:bad['objects'][1]['bytes']=513
            with self.assertRaises(GateFailure):snapshot_preserved(old,bad,{'sha256':'new'})
    def test_atomic_exclusive_state_publication(self):
        with tempfile.TemporaryDirectory() as raw:
            p=Path(raw)/'state.json';new_json(p,{'complete':True})
            self.assertEqual(json.loads(p.read_text()),{'complete':True})
            self.assertEqual(stat.S_IMODE(p.stat().st_mode),0o600)
            with self.assertRaises(FileExistsError):new_json(p,{'overwrite':True})
            self.assertEqual(json.loads(p.read_text()),{'complete':True})
            self.assertFalse(list(Path(raw).glob('.controller-stage-*')))
    def test_occupied_symlink_is_preserved(self):
        with tempfile.TemporaryDirectory() as raw:
            root=Path(raw);(root/'sentinel').write_text('original');(root/'state').symlink_to('sentinel')
            with self.assertRaises(FileExistsError):new_json(root/'state',{'bad':True})
            self.assertTrue((root/'state').is_symlink());self.assertEqual((root/'sentinel').read_text(),'original')
    def test_concurrent_state_publication_has_one_winner(self):
        with tempfile.TemporaryDirectory() as raw:
            p=Path(raw)/'winner';barrier=threading.Barrier(2);out=[]
            def race(n):
                barrier.wait()
                try:new_json(p,{'winner':n});out.append(True)
                except FileExistsError:out.append(False)
            ts=[threading.Thread(target=race,args=(i,)) for i in range(2)]
            for t in ts:t.start()
            for t in ts:t.join()
            self.assertEqual(sorted(out),[False,True]);self.assertIn(json.loads(p.read_text())['winner'],[0,1])

class ReviewBindingTests(unittest.TestCase):
    def decision(self,**change):
        cfg={'controller_pr':38,'controller_revision':'a'*40,'bundle_sha256':'b'*64,'probe_sha256':'c'*64}
        d={'kind':'SFD_CONTROLLER_ACCEPTANCE_V1','accepted':True,'revision':cfg['controller_revision'],'bundle_sha256':cfg['bundle_sha256'],
           'probe_sha256':cfg['probe_sha256'],'master':MASTER,'scope_plan_sha256':PINS['scope'],'operator_review_comment':6427391643}
        d.update(change);body='Aether independent decision\n```json\n'+json.dumps(d)+'\n```'
        c={'body':body,'user':{'login':'dreamingfree09'},'issue_url':'https://api.github.com/repos/dreamingfree09/secure-file-drop/issues/38'}
        cfg['controller_review_body_sha256']=hashlib.sha256(body.encode()).hexdigest()
        return c,cfg
    def test_exact_acceptance(self):
        c,cfg=self.decision();self.assertTrue(validate_review(c,cfg)['accepted'])
    def test_wrong_revisions_scope_or_unaccepted_rejected(self):
        for change in [{'accepted':False},{'revision':'d'*40},{'bundle_sha256':'e'*64},{'probe_sha256':'f'*64},
                       {'master':PROTOCOL_MASTER},{'scope_plan_sha256':'wrong'},{'operator_review_comment':6413982287}]:
            with self.subTest(change=list(change)):
                c,cfg=self.decision(**change)
                with self.assertRaises(GateFailure):validate_review(c,cfg)
    def test_wrong_author_thread_or_modified_body(self):
        for mutation in ['author','thread','body']:
            c,cfg=self.decision()
            if mutation=='author':c['user']['login']='unrelated'
            elif mutation=='thread':c['issue_url']='https://api.github.com/repos/dreamingfree09/secure-file-drop/issues/35'
            else:c['body']+='edited'
            with self.assertRaises(GateFailure):validate_review(c,cfg)
    def test_ambiguous_decisions_rejected(self):
        c,cfg=self.decision();c['body']+='\n'+c['body'];cfg['controller_review_body_sha256']=hashlib.sha256(c['body'].encode()).hexdigest()
        with self.assertRaises(GateFailure):validate_review(c,cfg)

class PrivateHandoffTests(unittest.TestCase):
    def buffer(self):return HandoffBuffer(json.dumps({'event':'ACCOUNT_ACTIVE','username':'sfd_acceptance_20261026','password':'Sfd9!'+'x'*43}).encode(),'sfd_acceptance_20261026')
    def test_secret_held_until_exact_success_verdict(self):
        b=self.buffer()
        with self.assertRaises(GateFailure):b.release({'success':True,'nonce':'wrong','actual_master':MASTER},'expected',MASTER)
        self.assertTrue(b.data)
        raw=b.release({'success':True,'nonce':'expected','actual_master':MASTER},'expected',MASTER)
        self.assertEqual(json.loads(raw)['username'],'sfd_acceptance_20261026');self.assertFalse(b.data)
    def test_denied_or_wrong_master_never_releases(self):
        for verdict in [{'success':False,'nonce':'n','actual_master':MASTER},{'success':True,'nonce':'n','actual_master':PROTOCOL_MASTER}]:
            b=self.buffer()
            with self.assertRaises(GateFailure):b.release(verdict,'n',MASTER)
            b.clear();self.assertFalse(b.data)
    def test_username_or_extra_secret_fields_rejected(self):
        for body in [{'event':'ACCOUNT_ACTIVE','username':'different','password':'x'*48},
                     {'event':'ACCOUNT_ACTIVE','username':'sfd_acceptance_20261026','password':'x'*48,'token':'extra'}]:
            with self.assertRaises(GateFailure):HandoffBuffer(json.dumps(body).encode(),'sfd_acceptance_20261026')
if __name__=='__main__':unittest.main()
