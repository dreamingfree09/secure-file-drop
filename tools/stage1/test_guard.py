import copy,datetime as dt,json,pathlib,tempfile,unittest
import guard
class GuardTests(unittest.TestCase):
    def positive(self):
        keys=['no_protected_byte_access','source_clean','source_manifest_verified','authority_exact','owner_go_exact','host_admin','HyperV_capable','host_RAM_available','host_commit_margin','host_identity_bound','maintenance_window','no_pending_reboot','H_identity_bound','H_ntfs','H_root_absent_or_owned','no_reparse','no_broad_lab_write','no_Docker_processes','no_WSL_workloads','HCS_query_complete','no_unknown_compute_systems','disk_inventory_complete','no_file_backed_unknown_disks','no_original_image_attachments','media_verified','media_rights_verified','runtime_bundle_verified','preventive_services_disabled_stopped','exclusive_activation_locks_verified','guardian_exact_binary_identities','native_lock_canary_verified','guardian_challenge_verified','HyperV_ready','fixed_disks_reserved','private_switch_verified','no_management_OS_adapter','two_guest_ids_bound','all_disks_new_owned','no_extra_guest_NICs','no_host_shares','no_default_gateway_DNS','no_external_route','guest_payload_verified','vm_autostart_nothing','checkpoint_off','secure_boot_vTPM_verified','H_recovery_reservation_verified','independent_supervisor_verified','service_snapshot_immutable','Docker_service_stopped','rollback_snapshot_ready']
        f={x:True for x in keys};f.update(schema='SFD_STAGE1_LIVE_FACTS_V1',collector='collect-host.ps1',C_free_GiB=20,H_free_GiB=280,RAM_GiB=64,age_seconds=0);return f
    def test_positive_is_not_authority(self):
        r=guard.evaluate(self.positive());self.assertEqual(r['decision'],'PASS_EVIDENCE');self.assertFalse(r['authorises_mutation'])
    def test_each_required_gate_missing_false_unknown_and_wrong_type(self):
        base=self.positive()
        for key,value in base.items():
            if key in ['service_snapshot_immutable','Docker_service_stopped','rollback_snapshot_ready']:continue
            for bad in ['MISSING',None,False,'true',[],{}]:
                with self.subTest(gate=key,bad=bad):
                    f=copy.deepcopy(base)
                    if bad=='MISSING':f.pop(key)
                    else:f[key]=bad
                    self.assertEqual(guard.evaluate(f)['decision'],'NO-GO')
    def test_preventive_gate_cannot_be_replaced_with_stopped_service(self):
        f=self.positive();f.pop('preventive_services_disabled_stopped');f['Docker_service_stopped']=True;self.assertEqual(guard.evaluate(f)['decision'],'NO-GO')
    def test_unknown_phase(self):self.assertEqual(guard.evaluate(self.positive(),'boot-anything')['decision'],'NO-GO')
    def test_numeric_boundaries_and_boolean_not_number(self):
        for key,minimum in [('C_free_GiB',10),('H_free_GiB',20),('RAM_GiB',24)]:
            for value in [minimum-0.01,True,False,'999',None]:
                f=self.positive();f[key]=value;self.assertEqual(guard.evaluate(f)['decision'],'NO-GO')
            f=self.positive();f[key]=minimum;self.assertEqual(guard.evaluate(f)['decision'],'PASS_EVIDENCE')
        for age in [-1,30.01,True,'0']:
            f=self.positive();f['age_seconds']=age;self.assertEqual(guard.evaluate(f)['decision'],'NO-GO')
    def test_protect_cannot_override_service_snapshot_or_running_Docker(self):
        for key in ['service_snapshot_immutable','Docker_service_stopped','rollback_snapshot_ready']:
            f=self.positive();f[key]=False;self.assertEqual(guard.evaluate(f,'protect')['decision'],'NO-GO')
    def test_only_new_lexical_children(self):
        root=guard.POLICY['root'];self.assertTrue(guard.inside(root+'\\linux\\disk.vhdx'))
        for path in [root,root+'-other\\x',root+'\\..\\x',root+'\\x:stream',root+'\\x.\\a',root+'/x',r'\\?\H:\x',r'X:\sealed.vhdx',r'C:\Users\original.vhdx']:
            self.assertFalse(guard.inside(path),path)
    def packet(self):
        now=dt.datetime(2026,11,2,12,tzinfo=dt.timezone.utc);p=dict(schema='SFD_STAGE1_ACCEPTANCE_V1',role='Owner',decision='GO',work_id=guard.POLICY['work_id'],source_sha='a'*40,manifest_sha256='b'*64,bundle_sha256='c'*64,host_identity_sha256='d'*64,issue=45,issued_utc=now.isoformat(),expires_utc=(now+dt.timedelta(hours=1)).isoformat(),effects=guard.POLICY['effects'],media_terms=True)
        return now,p
    def comment(self,p):return dict(author_id=94070428,issue=45,body='SFD-STAGE1-ACCEPTANCE\n'+json.dumps(p))
    def test_exact_acceptance(self):
        now,p=self.packet()
        with self.assertRaises(guard.Stop):guard.validate_packet(self.comment(p),'Owner','a'*40,45,now)
    def test_authority_negative_cases(self):
        now,p=self.packet()
        for key,value in [('role','Codex'),('decision','PREPARE'),('source_sha','c'*40),('work_id','other'),('issue',44),('media_terms',False),('effects',p['effects']+['host-trust']),('expires_utc',now.isoformat()),('expires_utc',(now+dt.timedelta(hours=9)).isoformat()),('issued_utc',(now+dt.timedelta(seconds=1)).isoformat()),('manifest_sha256','bad')]:
            q=copy.deepcopy(p);q[key]=value
            with self.subTest(key=key,value=value):
                with self.assertRaises(guard.Stop):guard.validate_packet(self.comment(q),'Owner','a'*40,45,now)
        for c in [dict(author_id=1,issue=45,body=self.comment(p)['body']),dict(author_id=94070428,issue=44,body=self.comment(p)['body']),dict(author_id=94070428,issue=45,body='Owner approved preparation only')]:
            with self.assertRaises(guard.Stop):guard.validate_packet(c,'Owner','a'*40,45,now)
    def test_json_duplicate_rejected(self):
        for raw in ['{"a":true,"a":false}','NaN','{"x":Infinity}']:
            with self.assertRaises(guard.Stop):guard.strict_json(raw)
    def test_manifest_corruption_and_path_escape(self):
        import hashlib
        with tempfile.TemporaryDirectory(prefix='sfd-stage1-fixture-') as d:
            root=pathlib.Path(d);(root/'x').write_bytes(b'public')
            m={'schema':'SFD_STAGE1_SOURCE_MANIFEST_V1','files':[{'path':'x','bytes':6,'SHA256':hashlib.sha256(b'public').hexdigest()}]}
            (root/'manifest.json').write_text(json.dumps(m));self.assertEqual(len(guard.verify_manifest(root)),64)
            (root/'x').write_bytes(b'changed')
            with self.assertRaises(guard.Stop):guard.verify_manifest(root)
            m['files'][0]['path']='../x';(root/'manifest.json').write_text(json.dumps(m))
            with self.assertRaises(guard.Stop):guard.verify_manifest(root)
    def test_port_and_budget_policy(self):
        self.assertEqual(guard.POLICY['new_issuer_CDP_port'],18082);self.assertNotIn('guest-root-trust',guard.POLICY['effects']);self.assertEqual(sum(guard.POLICY['H_reservation_GiB'].values()),168)
if __name__=='__main__':unittest.main(verbosity=2)
