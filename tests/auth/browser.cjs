const assert = require('node:assert/strict');
const fs = require('node:fs');
const { chromium } = require(process.env.SFD_AUTH_BROWSER_MODULE);
let stage="initialization";
(async()=>{
 const origin=process.argv[2];const u=new URL(origin);
 assert.equal(u.protocol,'https:');assert.equal(u.hostname,'localhost');assert.notEqual(u.port,'8443');
 const fixture=JSON.parse(fs.readFileSync(process.argv[3],'utf8'));assert.equal(fixture.Origin,origin);
 stage="browser launch";
 const browser=await chromium.launch({headless:true,chromiumSandbox:true});
 try{
  const context=await browser.newContext(); // Normal certificate validation; only the ephemeral CI fixture root is trusted.
  const page=await context.newPage();
  stage="normal HTTPS navigation";await page.goto(origin);
  stage="visible UI password entry";await page.locator('#username').fill(fixture.Username);
  await page.locator('#password').fill(fixture.Password);
  stage="actual UI login";await page.evaluate(()=>login());
  stage="identity readback";const me=await page.evaluate(async()=>{const r=await fetch('/me');return {status:r.status,body:await r.json()};});
  assert.equal(me.status,200);assert.equal(me.body.username,fixture.Username);assert.equal(me.body.is_admin,true);
  assert.equal(await page.locator('#appScreen').isVisible(),true);assert.equal(await page.locator('#adminSection').isVisible(),true);
  stage="readiness permissions and logout";const status=await page.evaluate(async()=>({ready:(await fetch('/ready')).status,admin:(await fetch('/admin/files')).status,logout:(await fetch('/logout',{method:'POST'})).status,me:(await fetch('/me')).status}));
  assert.deepEqual(status,{ready:200,admin:200,logout:200,me:401});
  stage="fixture credential cleanup";await page.evaluate(()=>{document.getElementById('password').value='';});
  console.log('BROWSER ACCEPTANCE PASS: actual historical UI login, admin identity, readiness, permissions and logout with normal HTTPS');
 } finally {await browser.close();}
})().catch(error=>{console.error('BROWSER DIAGNOSTIC: '+JSON.stringify({stage,kind:/^[A-Za-z]+$/.test(error.name)?error.name:'Error',certificate_error:/ERR_CERT/.test(error.message),sandbox_error:/sandbox/i.test(error.message)}));process.exitCode=1;});
