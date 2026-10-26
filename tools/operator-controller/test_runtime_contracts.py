"""Adapter contracts: no production command is executed."""
import ast,json,tempfile,unittest,os
from pathlib import Path
from unittest.mock import patch
from policy import GateFailure,PINS,MASTER,PROTOCOL_MASTER,new_json
import runtime

class DerivedDriverScopeTests(unittest.TestCase):
 def test_original_driver_is_immutable_and_only_authority_IPC_changes(self):
  root=Path(__file__).resolve().parents[2]
  original=root/'tools/operator-acceptance/staged_account.py'
  import hashlib
  self.assertEqual(hashlib.sha256(original.read_bytes()).hexdigest(),PINS['operator_driver'])
  old=ast.parse(original.read_text());new=ast.parse(Path(__file__).with_name('staged_driver.py').read_text())
  functions=lambda m:{n.name:ast.dump(n,include_attributes=False) for n in m.body if isinstance(n,(ast.FunctionDef,ast.ClassDef))}
  a,b=functions(old),functions(new);self.assertEqual(set(a),set(b))
  changed={k for k in a if a[k]!=b[k]};self.assertEqual(changed,{'decision','new_json'})
  values=lambda m:{n.targets[0].id:ast.literal_eval(n.value) for n in m.body if isinstance(n,ast.Assign) and len(n.targets)==1 and isinstance(n.targets[0],ast.Name) and isinstance(n.value,ast.Constant)}
  ov,nv=values(old),values(new)
  self.assertEqual({k for k in ov if ov[k]!=nv[k]},{'MASTER'})
  self.assertEqual(nv['MASTER'],MASTER);self.assertEqual(ov['MASTER'],PROTOCOL_MASTER)
 def test_no_production_shell_or_arbitrary_class_loader(self):
  r=Path(__file__).with_name('runtime.py').read_text()
  self.assertNotIn('shell=True',r);self.assertNotIn('docker compose down',r)
  main=Path(__file__).with_name('main.py').read_text();self.assertNotIn('import_module',main)
  self.assertNotIn('--unsafe',main)

class RuntimeMutationContractTests(unittest.TestCase):
 def runtime(self):
  r=object.__new__(runtime.Runtime);r.old_id='old';r.owner_pid=None;r.new_backend=None;r.handoff_released=False
  r.cfg={};r.base=[];r.pre={'synthetic':True};r.expected_env={'synthetic':'fixture'};return r
 def test_adoption_flags_and_backend_only(self):
  r=self.runtime();r.compose_args=['docker','compose','-p','securefiledrop']
  r.expected_env={'SFD_BACKUP_RETENTION_DAYS':'30'}
  old={'Mounts':[],'NetworkSettings':{'Networks':{'private':{}}}}
  new={'Name':'/sfd_backend','Id':'new','Image':runtime.IMAGE,'Config':{'StopTimeout':25,'Env':['SFD_BACKUP_RETENTION_DAYS=30']},
       'Mounts':[],'NetworkSettings':{'Networks':{'private':{}}}}
  r.pre={'old_backend':old};r.services=lambda:[new];r.other_services=lambda cs:None;r.now=lambda:1
  calls=[]
  with patch('runtime.command',side_effect=lambda args,**kw:calls.append(args) or b''):
   r.adopt()
  self.assertEqual(calls,[r.compose_args+['up','-d','--no-deps','--no-build','--pull','never','--force-recreate','backend']])
 def test_fail_closed_never_starts_recovery_or_old_backend(self):
  r=self.runtime();calls=[]
  r.inspect=lambda ident:{'Id':ident,'Image':runtime.IMAGE if ident=='new' else 'unexpected'}
  def command(args,**kw):
   calls.append(args)
   return b'new\nother\n' if args[1]=='ps' else b''
  with patch('runtime.command',side_effect=command):r.fail_closed_backend()
  self.assertIn(['docker','stop','--time','25','new'],calls)
  self.assertFalse(any('start' in c or 'up' in c or 'rm' in c for c in calls))
 def test_private_handoff_follows_exact_nonce_and_master(self):
  r=self.runtime()
  with tempfile.TemporaryDirectory() as raw:
   r.root=Path(raw);r.window_alive=lambda pid:None;r.owner_pid=42;r.now=lambda:1000
   ready={'nonce':'a'*48};result={'username':'sfd_acceptance_20261026'}
   new_json(r.root/'handoff-delivered.json',{'success':True,'username':result['username']})
   r.release_handoff(ready,result)
   v=json.loads((r.root/'handoff-approved.json').read_text())
   self.assertEqual(v['actual_master'],MASTER);self.assertEqual(v['nonce'],ready['nonce']);self.assertTrue(r.handoff_released);self.assertEqual(r.pre,{});self.assertEqual(r.expected_env,{})
 def test_disabled_context_does_no_external_work(self):
  r=self.runtime();r.cfg={'enabled':False}
  with patch('runtime.command',side_effect=AssertionError('no process allowed')):
   with self.assertRaises(GateFailure):r.authority()
if __name__=='__main__':unittest.main()
