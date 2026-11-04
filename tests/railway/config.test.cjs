const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');
const fixture=process.env.SFD_RAILWAY_IAC_FIXTURE;
assert.ok(fixture,'run the integrity-verifying tests/railway/check.py entry point');
const denied=()=>{throw new Error('external IO forbidden in IaC fixture');};
const sandbox={exports:{}, require(name){
  if(name==='crypto')return require('node:crypto');
  if(name==='child_process')return {execFileSync:denied};
  if(name==='process')return {env:{},argv:[],execArgv:[],cwd:()=>'/synthetic'};
  throw new Error('unexpected SDK dependency');
}};
vm.runInNewContext(fs.readFileSync(fixture,'utf8'),sandbox,{timeout:2000});
const sdk=sandbox.exports;
const input=fs.readFileSync(path.join(__dirname,'../../.railway/railway.ts'),'utf8');
const executable=input.replace(/^import[^\n]+;\n/,'').replace(/^export const partial[^\n]+;$/m,'').replace('export default','return');
const program=new Function('defineRailway','project','service','preserve',executable)(sdk.defineRailway,sdk.project,sdk.service,sdk.preserve);
test('only the explicitly named fresh staging scope is allowed',()=>{
  for(const context of [{projectName:'production',environment:'staging'},{projectName:'secure-file-drop-staging',environment:'production'},{}])assert.throws(()=>program(context),/STOP:/);
});
test('official SDK accepts a minimal Dockerfile backend with fixed matching port and readiness',()=>{
  const graph=program({projectName:'secure-file-drop-staging',environment:'staging'});
  assert.equal(graph.name,'secure-file-drop-staging');
  assert.equal(graph.resources.length,1);
  const backend=graph.resources[0];
  assert.equal(backend.name,'backend');
  const config=backend;
  assert.equal(config.build.builder,'DOCKERFILE');
  assert.equal(config.build.dockerfilePath,'Dockerfile.backend');
  assert.equal(config.deploy.healthcheckPath,'/ready');
  assert.equal(config.variables.PORT.value,'8080');
  assert.equal(config.variables.SFD_ADDR.value,':8080');
  assert.equal(config.variables.SFD_MAX_UPLOAD_BYTES.value,'10485760');
  assert.equal(config.variables.SFD_S3_BUCKET_LOOKUP.value,'virtual');
  assert.equal(config.variables.SFD_EMAIL_PROVIDER.value,'resend');
  assert.equal(config.source,undefined,'source/autodeploy binding requires separately authorised account context');
  assert.equal(config.networking,undefined,'no public endpoint is created by this source');
});

test('sensitive settings remain private Railway values and cleanup stays disabled',()=>{
  const backend=program({projectName:'secure-file-drop-staging',environment:'staging'}).resources[0];
  for(const key of ['DATABASE_URL','SFD_S3_ACCESS_KEY','SFD_S3_SECRET_KEY','SFD_ADMIN_PASS','SFD_SESSION_SECRET','SFD_DOWNLOAD_SECRET','SFD_RESEND_API_KEY','SFD_PUBLIC_BASE_URL'])assert.equal(JSON.stringify(backend.variables[key]),JSON.stringify(sdk.preserve()));
  assert.equal(backend.variables.SFD_CLEANUP_ENABLED.value,'false');
  assert.equal(backend.deploy.numReplicas,1);
  assert.equal(backend.deploy.healthcheckTimeout,300);
});

test('SDK dependency is locked and the shutdown budget preserves active-transfer handling',()=>{
  const root=path.join(__dirname,'../../.railway');
  const pkg=JSON.parse(fs.readFileSync(path.join(root,'package.json'),'utf8'));
  const lock=JSON.parse(fs.readFileSync(path.join(root,'package-lock.json'),'utf8'));
  assert.equal(pkg.dependencies.railway,'3.12.0');
  assert.equal(lock.packages['node_modules/railway'].version,'3.12.0');
  assert.equal(lock.packages['node_modules/railway'].integrity,'sha512-a6th+76NE3fZ8QMTOtEIe15sPTjObWOJYx0pCHCR/rd00r++I2rZHLKTn2R1qmPAwvGdAj6vY45GDVJs0AzOXA==');
  assert.equal(program({projectName:'secure-file-drop-staging',environment:'staging'}).resources[0].deploy.drainingSeconds,25);
});
