const assert = require('node:assert/strict');
const fs = require('node:fs');
const crypto = require('node:crypto');
const { chromium } = require(process.env.SFD_AUTH_BROWSER_MODULE);
let stage='fixture guard'; const checks=[];
function check(value,name){ assert.ok(value,name); checks.push(name); }
(async()=>{
 const origin=process.argv[2],u=new URL(origin),fixture=JSON.parse(fs.readFileSync(process.argv[3],'utf8'));
 assert.equal(u.protocol,'https:');assert.equal(u.hostname,'localhost');assert.notEqual(u.port,'8443');assert.equal(fixture.Origin,origin);
 assert.match(fixture.FixtureOwner,/^sfd-auth-[0-9a-f]{12}$/);assert.equal(fixture.Uploader,'uploader');assert.ok(fixture.UploaderPassword);
 const browser=await chromium.launch({headless:true,chromiumSandbox:true});
 try{
  const context=await browser.newContext({acceptDownloads:true,viewport:{width:1280,height:900}}),page=await context.newPage();
  // No CDN dependency or OS clipboard access is needed for these owned journeys.
  await context.route('**/*',route=>new URL(route.request().url()).origin===origin?route.continue():route.abort());
  await page.addInitScript(()=>{document.execCommand=command=>{if(command!=='copy')return false;window.fixtureCopied=document.getElementById('downloadLink').value;return true;};});
  const errors=[];page.on('pageerror',()=>errors.push('page error'));
  async function login(user,password){
   await page.locator('#username').fill(user);await page.locator('#password').fill(password);
   const response=page.waitForResponse(r=>r.url()===origin+'/login'&&r.request().method()==='POST');await page.evaluate(()=>login());
   check((await response).status()===200,'actual '+(user==='admin'?'environment administrator':'registered user')+' UI login');
  }
  stage='admin login and quota';await page.goto(origin);await login(fixture.Username,fixture.Password);
  await page.waitForFunction(()=>!document.getElementById('appScreen').classList.contains('hidden'));
  check(await page.locator('#password').inputValue()==='','sign-in password field cleared');
  await page.waitForFunction(()=>document.getElementById('quotaOutput').textContent.includes('No personal storage quota'));
  check(await page.locator('#quotaBanner').evaluate(el=>el.classList.contains('hidden')),'environment administrator quota is inapplicable, not a temporary failure');
  check(await page.locator('#adminSection').isVisible(),'environment administrator dashboard visible');
  const admin=await page.evaluate(async()=>{const r=await fetch('/me');return {status:r.status,body:await r.json()};});
  check(admin.status===200&&admin.body.is_admin===true&&admin.body.username==='admin','actual admin identity verified');
  await page.evaluate(()=>logout());await page.waitForFunction(()=>!logoutPending);
  check((await page.request.get(origin+'/me')).status()===401,'actual environment admin logout revokes session');
  stage='registered user quota and uploads';await login(fixture.Uploader,fixture.UploaderPassword);
  await page.waitForFunction(()=>!document.getElementById('appScreen').classList.contains('hidden'));
  await page.waitForFunction(()=>document.getElementById('quotaOutput').textContent.includes('Storage Used'));
  check(await page.locator('#quotaBanner').evaluate(el=>el.classList.contains('hidden')),'registered user quota readback works');
  check(!await page.locator('#adminSection').isVisible(),'ordinary user administrator dashboard hidden');
  const samples=[{name:'UI Δοκιμή 世界 with spaces.txt',data:Buffer.from('Synthetic legacy browser data\n')},{name:'UI empty space.txt',data:Buffer.alloc(0)}];
  for(const [i,sample] of samples.entries()){
   stage='real upload '+i;const uploaded=page.waitForResponse(r=>new URL(r.url()).pathname==='/upload'&&r.request().method()==='POST');
   await page.locator('#fileInput').setInputFiles({name:sample.name,mimeType:'text/plain',buffer:sample.data});check((await uploaded).status()===200,'actual multipart upload '+i);
   await page.waitForFunction(()=>document.getElementById('linkContainer').classList.contains('show'));
   const link=await page.locator('#downloadLink').inputValue();check(new URL(link).origin===origin&&new URL(link).pathname==='/download','owned HTTPS signed link '+i);
   const downloaded=await page.request.get(link);check(downloaded.status()===200&&(await downloaded.body()).equals(sample.data),'actual byte-equal download '+i);
   const rows=await page.evaluate(async()=>(await fetch('/user/files')).json()),row=rows.find(x=>x.orig_name===sample.name);
   check(row&&row.sha256_hex===crypto.createHash('sha256').update(sample.data).digest('hex'),'native hash and owned browser metadata '+i);
   await page.getByRole('button',{name:'Copy Link',exact:true}).click();check(await page.evaluate(()=>window.fixtureCopied===document.getElementById('downloadLink').value),'copy control with isolated test clipboard '+i);
  }
  stage='logout clears transfer state';await page.evaluate(()=>logout());await page.waitForFunction(()=>!logoutPending);
  check(await page.locator('#downloadLink').inputValue()==='','logout clears signed link immediately');
  check(await page.locator('#fileInput').inputValue()===''||await page.locator('#appScreen').evaluate(el=>el.classList.contains('hidden')),'signed-out application hidden');
  check((await page.request.get(origin+'/me')).status()===401,'registered logout rejects protected readback');
  stage='unverified session cannot enter UI';
  await page.route(origin+'/me',route=>route.fulfill({status:500,contentType:'text/plain',body:'synthetic unavailable identity'}));
  await login(fixture.Uploader,fixture.UploaderPassword);
  await page.waitForFunction(()=>document.getElementById('loginAlert').textContent.includes('error')||document.getElementById('loginAlert').textContent.includes('Connection'));
  check(await page.locator('#appScreen').evaluate(el=>el.classList.contains('hidden')),'login200 plus identity500 does not accept a session');
  await page.unroute(origin+'/me');await page.request.post(origin+'/logout',{headers:{Origin:origin},data:{}});
  stage='expired or missing session UI';await login(fixture.Uploader,fixture.UploaderPassword);
  await page.waitForFunction(()=>!document.getElementById('appScreen').classList.contains('hidden'));await context.clearCookies();await page.evaluate(()=>loadMyFiles());
  await page.waitForFunction(()=>document.getElementById('appScreen').classList.contains('hidden'));
  check(await page.locator('#downloadLink').inputValue()==='','401 resets account and protected transfer UI');
  check(errors.length===0,'no uncaught browser errors');
  await context.close();console.log('BROWSER ACCEPTANCE PASS: actual historical browser UI with verified identities, role-correct quota, upload/hash/download and session-state cleanup');
  console.log('LEGACY BROWSER CHECKS: '+JSON.stringify({count:checks.length,checks,clipboard:'isolated test clipboard; OS clipboard unexecuted',external_QR:'external requests blocked in synthetic fixture; QR integration unexecuted'}));
 }finally{await browser.close();}
})().catch(error=>{console.error('BROWSER DIAGNOSTIC: '+JSON.stringify({stage,kind:error.name,certificate_error:/CERT_|certificate/i.test(error.message),assertion:error.name==='AssertionError'?error.message:'runtime failure; values redacted'}));process.exitCode=1;});