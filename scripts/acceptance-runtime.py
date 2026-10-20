#!/usr/bin/env python3
"""Run existing owned suites; publish only credential-free structured evidence."""
import json, os, pathlib, subprocess, sys, re
os.umask(0o077)
suite=sys.argv[1]
assert suite in ('default','contract','rehearsal')
test='TestRustFSMigrationRehearsal' if suite=='rehearsal' else 'TestUploadHashDownloadFlow'
expected={'default':15,'contract':18,'rehearsal':6}[suite]
env=dict(os.environ,SFD_E2E='1',SFD_BROWSER='1')
if suite=='contract':env['SFD_STORAGE_EVALUATION']='rustfs'
if suite=='default':env['SFD_DESKTOP_CLIENT']='1'
if suite=='rehearsal':env.update(SFD_RUSTFS_REHEARSAL='1',SFD_REHEARSAL_REQUIRED='1',SFD_REHEARSAL_EVIDENCE=str(pathlib.Path(os.environ['RUNNER_TEMP'])/'rehearsal-private'))
root=pathlib.Path(os.environ['RUNNER_TEMP']); log=root/'runtime-private.jsonl'
if suite=='rehearsal':pathlib.Path(env['SFD_REHEARSAL_EVIDENCE']).mkdir(mode=0o700,exist_ok=False)
with log.open('xb') as out:process=subprocess.run(['go','test','./tests/e2e','-json','-v','-run','^'+test+'$','-count=1','-timeout','29m'],env=env,stdout=out,stderr=subprocess.STDOUT)
events=[]
for line in log.read_text().splitlines():
 try:events.append(json.loads(line))
 except json.JSONDecodeError:pass
passed=[x['Test'] for x in events if x.get('Action')=='pass' and x.get('Test','').startswith(test+'/')]
failures=[x.get('Test',x.get('Package')) for x in events if x.get('Action')=='fail'];skips=[x.get('Test',x.get('Package')) for x in events if x.get('Action')=='skip']
output=''.join(x.get('Output','') for x in events);browser=[]
for line in output.splitlines():
 if 'BROWSER CHECKS: ' in line:browser.append(json.loads(line.split('BROWSER CHECKS: ',1)[1]))
desktop=output.count('DESKTOP PASS:')
diagnostics=[line.strip() for line in output.splitlines() if re.search(r'(Alpine secdb HTTP [0-9]+|SBOM.*HTTP [0-9]+|unsupported runtime distribution|scanner failed:|invalid scanner JSON|storage binary scan timed out)',line)]
result={'diagnostics':diagnostics,'revision':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),'suite':suite,'exit':process.returncode,'expected_scenarios':expected,'passed':passed,'failures':failures,'skips':skips,'browser':browser,'portable_desktop_checks':desktop,'real_Owner_GUI':'unexecuted','private_log_published':False}
(root/'acceptance.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result))
assert process.returncode==0 and len(passed)==expected and not failures and not skips,'mandatory suite did not pass'
assert len(browser)==1 and browser[0]['count']==37,'mandatory browser journey missing'
if suite=='default':assert desktop==17 and 'DESKTOP REAL BACKEND CHECKS PASSED: 17' in output,'mandatory portable client journey missing'
