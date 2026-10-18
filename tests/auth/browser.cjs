const assert = require('node:assert/strict');
const { chromium } = require(process.env.SFD_AUTH_BROWSER_MODULE);
(async()=>{
 const origin=process.argv[2];const u=new URL(origin);
 assert.equal(u.protocol,'https:');assert.equal(u.hostname,'localhost');assert.notEqual(u.port,'8443');
 const browser=await chromium.launch({headless:true,chromiumSandbox:true});
 try{
  const context=await browser.newContext(); // Normal certificate validation; fixture root is trusted only in the ephemeral CI runner.
  const page=await context.newPage();
  await page.goto(origin+'/__sfd-owned-browser-test');
  await page.waitForFunction(()=>document.getElementById('result').textContent.startsWith('BROWSER ACCEPTANCE PASS:'),{},{timeout:20000});
  console.log('BROWSER ACCEPTANCE PASS: actual historical UI login, admin identity, readiness, permissions and logout with normal HTTPS');
 } finally {await browser.close();}
})().catch(error=>{console.error('BROWSER ACCEPTANCE FAIL: '+error.name);process.exitCode=1;});