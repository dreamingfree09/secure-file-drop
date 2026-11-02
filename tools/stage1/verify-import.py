"""Inside the identity-checked new Linux guest only: compare imported config IDs."""
import json,pathlib,subprocess,sys
r=json.loads(pathlib.Path(sys.argv[1]).read_text())
assert r['schema']=='SFD_STAGE1_RUNTIME_RECEIPT_V1' and r['application']=='61195434bee5f9221a6f55469c719cb360504ea9'
for name,expected in r['exported_image_ids'].items():
    got=json.loads(subprocess.check_output(['docker','--host','unix:///run/sfd-lab/docker.sock','image','inspect',expected]))[0]
    assert got['Id']==expected
lock=r['candidate_locks']['images']
roles={'BACKEND':'sfd-stage1-backend','POSTGRES':lock['postgres'],'STORAGE':lock['rustfs'],'PROXY':lock['traefik']}
out=pathlib.Path('/srv/sfd-lab/private/images.env')
with out.open('x') as f:
    for role,name in roles.items():f.write('SFD_LAB_'+role+'_IMAGE='+r['exported_image_ids'][name]+'\n')
print('{"all_imported_config_identities_match":true}')
