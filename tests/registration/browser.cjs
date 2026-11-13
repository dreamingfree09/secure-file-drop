const assert=require('node:assert/strict');
const {createHash}=require('node:crypto');
const {readFileSync}=require('node:fs');
const {join}=require('node:path');
const {chromium}=require('../browser/node_modules/playwright-core');

async function main(){
  const root=process.env.R1_RUNTIME,origin='https://sfd.fixture.test:'+process.env.R1_ORIGIN_PORT;
  const password=process.env.SFD_R1_FIXTURE_USER_PASSWORD;
  const payload=Buffer.from('Administrator-invited browser account; native file integrity.\n'.repeat(59));
  const browser=await chromium.launch({env:{...process.env,HOME:join(root,'r4-browser-home')},args:['--host-resolver-rules=MAP sfd.fixture.test 127.0.0.1','--no-proxy-server']});
  const adminContext=await browser.newContext({viewport:{width:1280,height:900}});
  const recipientContext=await browser.newContext({viewport:{width:390,height:844},acceptDownloads:true});
  const admin=await adminContext.newPage(),recipient=await recipientContext.newPage();
  const errors=[];for(const page of [admin,recipient])page.on('pageerror',err=>errors.push(err.name));
  const receipts=[],paths=new Set(['/login','/me','/config','/files','/upload','/links','/quota','/user/files','/logout','/invitations','/admin/registration-invitations','/registration-invitations/redeem']);
  for(const [name,page] of [['admin',admin],['recipient',recipient]])page.on('response',response=>{
    const url=new URL(response.url());if(url.origin!==origin||!paths.has(url.pathname))return;
    receipts.push({page:name,path:url.pathname,status:response.status()});if(receipts.length>64)receipts.shift();
  });
  const api=async(page,path,method='GET',body)=>page.evaluate(async({path,method,body})=>{
    const response=await fetch(path,{method,headers:body?{'Content-Type':'application/json'}:{},body:body?JSON.stringify(body):undefined});
    return {status:response.status,body:await response.text()};
  },{path,method,body});
  const login=async(page,username,pass)=>{
    await page.goto(origin);await page.waitForFunction(()=>!logoutPending);
    for(let attempt=0;attempt<2;attempt++){
      await page.locator('#username').fill(username);await page.locator('#password').fill(pass);
      const receipt=page.waitForResponse(r=>new URL(r.url()).pathname==='/login');
      await page.getByRole('button',{name:'Sign In',exact:true}).click();const response=await receipt;
      if(response.status()===200)break;
      assert.equal(response.status(),429);assert.equal(attempt,0);assert.equal(response.headers()['retry-after'],'60');
      console.log('Browser fixture honours observed429 and Retry-After60');await page.waitForTimeout(61000);
    }
    await page.locator('#logoutBtn').waitFor({state:'visible'});
  };
  const adminReady=async()=>{await admin.locator('#registrationAdminPanel').waitFor({state:'visible'});await admin.waitForFunction(()=>!document.querySelector('#registrationRefresh').disabled);};
  const issue=async()=>{
    const receipt=admin.waitForResponse(r=>new URL(r.url()).pathname==='/admin/registration-invitations'&&r.request().method()==='POST');
    await admin.locator('#registrationIssue').click();const response=await receipt;assert.equal(response.status(),201);
    const value=await response.json();await admin.locator('#registrationReceipt').waitFor({state:'visible'});await adminReady();
    assert.equal(await admin.locator('#registrationURL').inputValue(),value.url);
    return value;
  };
  const fill=async(page,username)=>{
    await page.locator('#joinStart').click();
    await page.locator('#joinUsername').fill(username);await page.locator('#joinEmail').fill(username+'@example.invalid');
    await page.locator('#joinPassword').fill(password);await page.locator('#joinConfirm').fill(password);
  };
  const submit=async(page,expected)=>{
    for(let attempt=0;attempt<2;attempt++){
      const receipt=page.waitForResponse(r=>new URL(r.url()).pathname==='/registration-invitations/redeem');
      await page.locator('#joinSubmit').click();const response=await receipt;
      if(response.status()!==429){assert.equal(response.status(),expected);assert.equal(response.headers()['set-cookie'],undefined);break;}
      assert.equal(attempt,0);assert.equal(response.headers()['retry-after'],'60');
      console.log('Recipient fixture honours observed429 and Retry-After60');await page.waitForTimeout(61000);
      await page.locator('#joinPassword').fill(password);await page.locator('#joinConfirm').fill(password);
    }
    await page.waitForFunction(()=>document.querySelector('#joinPassword').value==='');
  };
  try{
    if(process.argv[2]==='restore'){
      await login(recipient,'r4_browser_user',password);
      const identity=await api(recipient,'/me');assert.equal(JSON.parse(identity.body).is_admin,false);
      assert.equal((await api(recipient,'/admin/registration-invitations')).status,403);
      const files=JSON.parse((await api(recipient,'/user/files')).body);
      const file=files.find(file=>file.orig_name==='r4-browser-owned.txt');assert.ok(file);
      assert.equal(file.sha256_hex,createHash('sha256').update(payload).digest('hex'));
      await login(admin,'admin',process.env.SFD_R1_FIXTURE_PASSWORD);await adminReady();
      assert.ok(await admin.locator('.registration-badge-redeemed').count());
      console.log('PASS: real Chromium invited-account login, owner file digest and invitation states survive paired restore');
      return;
    }
    await login(admin,'admin',process.env.SFD_R1_FIXTURE_PASSWORD);await adminReady();
    await adminContext.grantPermissions(['clipboard-read','clipboard-write'],{origin});
    const first=await issue();const token=new URLSearchParams(new URL(first.url).hash.slice(1)).get('token');
    await admin.locator('#registrationCopy').click();assert.equal(await admin.evaluate(()=>navigator.clipboard.readText()),first.url);
    // Depart with a copied receipt, revoke that session in another tab, and
    // return through history. Cached client flags cannot revive authority.
    await admin.goto(origin+'/join');
    const otherTab=await adminContext.newPage();
    try{await otherTab.goto(origin);assert.equal((await api(otherTab,'/logout','POST')).status,200);}finally{await otherTab.close();}
    await admin.goBack();await admin.locator('#loginScreen').waitFor({state:'visible'});
    assert.equal(await admin.locator('#registrationURL').inputValue(),'');assert.equal(await admin.locator('#registrationAdminPanel').isVisible(),false);
    await login(admin,'admin',process.env.SFD_R1_FIXTURE_PASSWORD);await adminReady();
    const requests=[];
    recipient.on('request',request=>requests.push({url:request.url(),method:request.method()}));
    const preview=await recipient.goto(first.url);assert.equal(preview.status(),200);
    assert.equal((await preview.text()).includes(token),false);
    assert.equal(await recipient.evaluate(()=>location.hash),'');
    assert.equal(requests.some(r=>r.method==='POST'),false);
    assert.equal(requests.some(r=>r.url.includes(token)||new URL(r.url).origin!==origin),false);
    await recipient.locator('#joinStart').click();
    assert.equal(requests.some(r=>r.method==='POST'),false);
    assert.equal(await recipient.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
    await recipient.screenshot({path:join(root,'r4-join-mobile.png'),fullPage:true});
    await recipient.locator('#joinUsername').fill('r4_browser_user');await recipient.locator('#joinEmail').fill('r4_browser_user@example.invalid');
    await recipient.locator('#joinPassword').fill(password);await recipient.locator('#joinConfirm').fill(password);
    await submit(recipient,201);
    await recipient.getByText('Your account is ready. Sign in with your chosen username and password. Your contact email remains unverified.',{exact:true}).waitFor();
    assert.equal((await recipientContext.cookies()).some(c=>c.name==='sfd_session'),false);
    await recipient.screenshot({path:join(root,'r4-join-success.png'),fullPage:true});
    await login(recipient,'r4_browser_user',password);
    const identity=JSON.parse((await api(recipient,'/me')).body);assert.equal(identity.is_admin,false);assert.equal(identity.environment_admin,false);
    assert.equal(await recipient.locator('#registrationAdminPanel').isVisible(),false);
    assert.equal(JSON.parse((await api(recipient,'/quota')).body).storage_quota_bytes,10*1024**3);
    assert.equal((await api(recipient,'/admin/registration-invitations')).status,403);
    assert.equal((await api(recipient,'/admin/registration-invitations','POST',{ttl_seconds:3600})).status,403);
    await recipient.locator('#fileInput').setInputFiles({name:'r4-browser-owned.txt',mimeType:'text/plain',buffer:payload});
    await recipient.locator('#linkContainer.show').waitFor();
    const link=await recipient.locator('#downloadLink').inputValue();
    const files=JSON.parse((await api(recipient,'/user/files')).body);
    assert.equal(files.some(file=>file.orig_name==='rorschach-fixture.txt'),false);
    const own=files.find(file=>file.orig_name==='r4-browser-owned.txt');assert.ok(own);
    assert.equal(own.sha256_hex,createHash('sha256').update(payload).digest('hex'));
    const restricted=await api(recipient,'/invitations','POST',{file_id:own.id,delivery:'manual',ttl_hours:1});
    assert.equal(restricted.status,404,'unverified contact must retain protected-delivery restriction');
    const downloadPage=await browser.newPage({acceptDownloads:true});
    try{await downloadPage.goto(link);const saved=downloadPage.waitForEvent('download');await downloadPage.getByRole('button',{name:'Download file',exact:true}).click();assert.deepEqual(readFileSync(await(await saved).path()),payload);}finally{await downloadPage.close();}
    await admin.locator('#registrationRefresh').click();await adminReady();
    await admin.locator('[data-invitation-id="'+first.id+'"]').locator('.registration-badge-redeemed').waitFor();
    const revoked=await issue();const card=admin.locator('[data-invitation-id="'+revoked.id+'"]');
    await card.getByRole('button',{name:'Revoke invitation',exact:true}).click();
    await admin.locator('#registrationRevokeCancel').click();
    await card.getByRole('button',{name:'Revoke invitation',exact:true}).click();await admin.locator('#registrationRevokeYes').click();
    await card.locator('.registration-badge-revoked').waitFor();
    assert.equal(await admin.locator('#registrationURL').inputValue(),'');
    const denied=await browser.newPage();
    try{await denied.goto(revoked.url);await fill(denied,'r4_browser_revoked');await submit(denied,410);await denied.getByText('This invitation is unavailable, expired or already used. Ask the administrator for a new link.',{exact:true}).waitFor();
      await denied.goto(first.url);await fill(denied,'r4_browser_replay');await submit(denied,410);
    }finally{await denied.close();}
    await admin.screenshot({path:join(root,'r4-admin-desktop.png'),fullPage:true,mask:[admin.locator('#registrationReceipt')]});
    await admin.setViewportSize({width:390,height:844});assert.equal(await admin.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),true);
    await admin.screenshot({path:join(root,'r4-admin-mobile.png'),fullPage:true,mask:[admin.locator('#registrationReceipt')]});
    await admin.route('**/admin/registration-invitations',route=>route.abort('connectionreset'));
    await admin.locator('#registrationRefresh').click();await admin.getByText('Could not verify invitations. Refresh to try again.',{exact:true}).waitFor();
    assert.equal(await admin.locator('.registration-card').count(),0);await admin.unroute('**/admin/registration-invitations');
    await admin.locator('#logoutBtn').click();await admin.locator('#loginScreen').waitFor({state:'visible'});
    assert.equal(await admin.locator('#registrationAdminList').textContent(),'');assert.equal(await admin.locator('#registrationURL').inputValue(),'');
    await login(admin,'fixture',password);assert.equal(await admin.locator('#registrationAdminPanel').isVisible(),false);
    assert.equal(await admin.locator('#registrationAdminList').textContent(),'');
    assert.deepEqual(errors,[]);
    console.log('PASS: real Chromium administrator issue/copy, fragment-private preview, explicit no-email registration and ordinary username/password login');
    console.log('PASS: native-hashed upload/download bytes, quota/role/owner/email restrictions, invitation replay/revocation, mobile/desktop and logout isolation');
  }catch(error){
    const state=await recipient.evaluate(()=>({authenticated:isLoggedIn,generation:sessionGeneration,uploading:uploadInProgress,
      selected:!!selectedFile,current:!!currentUpload,queued:uploadQueue.length,receipt:document.querySelector('#linkContainer')?.classList.contains('show')===true})).catch(()=>({documentUnavailable:true}));
    console.error('Bounded fixed-path HTTP receipts:',JSON.stringify(receipts));console.error('Synthetic upload state:',JSON.stringify(state));throw error;
  }finally{await adminContext.close();await recipientContext.close();await browser.close();}
}
main().catch(error=>{
  // Report only owned source coordinates: Playwright call logs can contain URLs.
  const frames=[...String(error.stack).matchAll(/tests[\\/]registration[\\/]browser\.cjs:(\d+):(\d+)/g)].map(match=>match[1]+':'+match[2]);
  console.error('Registration browser acceptance failed:',error.name,'owned source frames',frames.join(','));process.exitCode=1;
});
