"""Render only a synthetic Compose configuration. Never starts services or reads .env."""
from pathlib import Path
import json,os,re,secrets,subprocess,tempfile
root=Path(__file__).resolve().parents[2]
template=root/'docker-compose.yml';raw=template.read_text();names=set(re.findall(r'\$\{([A-Z0-9_]+)',raw))
required=['POSTGRES_PASSWORD','MINIO_ROOT_PASSWORD','SFD_S3_ACCESS_KEY','SFD_S3_SECRET_KEY','SFD_ADMIN_PASS','SFD_SESSION_SECRET','SFD_DOWNLOAD_SECRET','GRAFANA_ADMIN_PASSWORD','DATABASE_URL']
checks=[]
def check(value,label):
 if not value:raise AssertionError(label)
 checks.append(label)
env={k:v for k,v in os.environ.items() if k not in names}
with tempfile.TemporaryDirectory(prefix='sfd-compose-contract-') as d:
 path=Path(d);marker='sfd-config-'+secrets.token_hex(8);(path/'owner').write_text(marker)
 values={k:'owned-synthetic-'+k.lower() for k in names};values.update(SFD_MAX_UPLOAD_BYTES='1024',SFD_MAX_REQUEST_BYTES=str(1024+(1<<20)))
 def render(omit=None):
  file=path/('complete.env' if omit is None else omit+'.env');file.write_text(''.join(k+'='+v+'\n' for k,v in sorted(values.items()) if k!=omit))
  return subprocess.run(['docker','compose','--project-name',marker,'--file',str(template),'--env-file',str(file),'config','--format','json'],env=env,capture_output=True,timeout=30)
 rendered=render();check(rendered.returncode==0,'synthetic Compose template renders without existing .env')
 doc=json.loads(rendered.stdout);backend=doc['services']['backend']
 check(backend.get('stop_grace_period')=='25s','Compose reserves tested25s backend stop budget')
 cleanup={'SFD_CLEANUP_ENABLED':'false','SFD_CLEANUP_INTERVAL':'17m','SFD_CLEANUP_MAX_AGE':'49h'}
 values.update(cleanup);off=json.loads(render().stdout)['services']['backend']['environment']
 check(all(off[k]==v for k,v in cleanup.items()),'disabled cleanup forwards explicit retention parameters')
 values['SFD_CLEANUP_ENABLED']='true';on=json.loads(render().stdout)['services']['backend']['environment']
 check(on['SFD_CLEANUP_ENABLED']=='true' and on['SFD_CLEANUP_INTERVAL']=='17m' and on['SFD_CLEANUP_MAX_AGE']=='49h','enabled cleanup forwards explicit retention parameters')
 for k in cleanup:values.pop(k)
 defaults=json.loads(render().stdout)['services']['backend']['environment']
 check(all(defaults[k]==v for k,v in {'SFD_CLEANUP_ENABLED':'false','SFD_CLEANUP_INTERVAL':'1h','SFD_CLEANUP_MAX_AGE':'24h'}.items()),'omitted cleanup uses disabled conservative defaults')

 check(backend['labels']['traefik.http.middlewares.sfd-buf.buffering.maxRequestBodyBytes']==str(1024+(1<<20)) and backend['environment']['SFD_MAX_REQUEST_BYTES']==str(1024+(1<<20)),'proxy and backend retain complete multipart envelope')
 check(render('SFD_MAX_REQUEST_BYTES').returncode!=0,'missing multipart envelope fails closed')
 mounts=[Path(v['source']) for service in doc['services'].values() for v in service.get('volumes',[]) if v.get('type')=='bind' and str(v.get('source','')).startswith(str(root))]
 check(bool(mounts) and all(v.exists() for v in mounts),'all referenced project configuration mounts exist')
 check(all(p.get('host_ip')=='127.0.0.1' for service in doc['services'].values() for p in service.get('ports',[])),'all template published ports are loopback-only')
 for k in ['SFD_S3_ACCESS_KEY','SFD_S3_SECRET_KEY']:check(backend['environment'][k]==values[k],'backend uses explicit existing storage variable '+k)
 for k in required:check(render(k).returncode!=0,'missing credential/configuration fails closed '+k)
 sample=dict(line.split('=',1) for line in (root/'.env.example').read_text().splitlines() if re.match('[A-Z][A-Z0-9_]*=',line))
 check(all(sample.get(k)=='' for k in required),'sample contains no usable default credential or connection identity')
 assert (path/'owner').read_text()==marker
print(json.dumps({'suite':'synthetic-compose-contract','checks':checks,'count':len(checks),'services_started':False,'existing_env_read':False,'real_secrets_used':False}))
