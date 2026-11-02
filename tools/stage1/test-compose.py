"""Config-only check on a disposable hosted runner. Never starts a service."""
import json,os,pathlib,subprocess
assert os.environ.get('GITHUB_ACTIONS')=='true' and os.environ.get('RUNNER_ENVIRONMENT')=='github-hosted' and os.name!='nt'
root=pathlib.Path('/srv/sfd-lab/private');assert not root.parent.exists()
subprocess.run(['sudo','install','-d','-m','700','-o',str(os.getuid()),'-g',str(os.getgid()),str(root)],check=True)
for name in ['postgres.env','storage.env','backend.env']:(root/name).write_text('')
env=dict(os.environ)
for role in ['POSTGRES','STORAGE','BACKEND','PROXY']:env['SFD_LAB_'+role+'_IMAGE']='sha256:'+('a'*64)
raw=subprocess.check_output(['docker','compose','-f','tools/stage1/compose.yml','config','--format','json'],env=env)
c=json.loads(raw);assert len(c['services'])==4
for v in c['services'].values():
    assert v['restart']=='no' and v['pull_policy']=='never' and v['image']=='sha256:'+('a'*64)
    assert 'ALL' in v['cap_drop'] and int(v['pids_limit'])>0 and int(v['mem_limit'])>0
    assert not any('/var/run/docker' in str(x) or 'original' in str(x) for x in v.get('volumes',[]))
b=c['services']['backend']['environment'];assert b['SFD_CLEANUP_ENABLED']=='false' and b['SFD_BACKUP_INTERVAL']=='24h' and b['SFD_BACKUP_RETENTION_DAYS']=='30'
assert c['networks']['private']['internal'] is True
print('{"configuration_only":true,"services":4,"engine_invoked":false,"all_contracts_pass":true}')
