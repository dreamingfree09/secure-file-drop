const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const crypto = require('node:crypto');
const path = require('node:path');
const { chromium } = require('playwright-core');
let stage = 'fixture guard';
const checks = [];
function check(value, name) { assert.ok(value, name); checks.push(name); }
(async () => {
  const base = process.argv[2];
  const parsed = new URL(base);
  assert.equal(parsed.protocol, 'http:'); assert.equal(parsed.hostname, '127.0.0.1');
  assert.match(process.env.SFD_BROWSER_RUN || '', /^sfd-quality-[0-9a-f]{16}$/);
  const fixture = JSON.parse(require('node:fs').readFileSync(0, 'utf8'));
  assert.equal(fixture.username, 'admin'); assert.ok(fixture.password);
  const fileLimit=Number(fixture.max_file_bytes || 1024); assert.ok(Number.isSafeInteger(fileLimit) && fileLimit>=1024 && fileLimit<=8*1024*1024);
  const browser = await chromium.launch({ headless: true, chromiumSandbox: true });
  try {
    const context = await browser.newContext({ acceptDownloads: true, viewport: { width: 1280, height: 900 } });
    const page = await context.newPage(); const pageErrors = [];
    page.on('pageerror', () => pageErrors.push('page error'));
    // Keep clipboard checks inside this disposable page. Never replace the
    // Owner's OS clipboard; that actual native interaction remains an Owner gate.
    await page.addInitScript(() => {
      document.execCommand = command => {
        if (command !== 'copy') return false;
        window.fixtureCopied = document.querySelector('#downloadLink').value; return true;
      };
    });
    const user = 'browser_' + crypto.randomBytes(6).toString('hex');
    const password = fixture.browser_password || ('SyntheticBrowser' + crypto.randomBytes(16).toString('hex') + '7');
    stage = 'initial page and accessibility'; await page.goto(base);
    check(await page.locator('#loginForm').getByLabel('Username', { exact: true }).count() === 1 && await page.locator('#loginForm').getByLabel('Password', { exact: true }).getAttribute('type') === 'password', 'initial page and labelled masked credentials');
    await page.getByRole('link', { name: 'Create Account', exact: true }).click();
    await page.getByLabel('Email Address').fill(user + '@example.test');
    await page.getByLabel('Username', { exact: true }).last().fill(user);
    await page.locator('#regPassword').fill(password); await page.locator('#regPasswordConfirm').fill(password);
    stage = 'real registration'; const registration = page.waitForResponse(r => r.url() === base + '/register' && r.request().method() === 'POST');
    await page.getByRole('button', { name: 'Create Account', exact: true }).click();
    check((await registration).status() === 201, 'actual UI synthetic registration');
    await page.waitForFunction(() => !document.getElementById('loginForm').classList.contains('hidden'));
    check((await Promise.all(['regEmail','regUsername','regPassword','regPasswordConfirm'].map(id => page.locator('#'+id).inputValue()))).every(x => x === ''), 'registration clears account and password fields');
    async function login(username, secret) {
      await page.locator('#username').fill(username); await page.locator('#password').fill(secret);
      const response = page.waitForResponse(r => r.url() === base + '/login' && r.request().method() === 'POST');
      await page.getByRole('button', { name: 'Sign In', exact: true }).focus(); await page.keyboard.press('Enter');
      return (await response).status();
    }
    stage = 'invalid credentials'; check(await login(user, 'incorrect-synthetic-only') === 401, 'UI rejects incorrect credentials');
    await page.waitForFunction(() => document.getElementById('loginAlert').textContent.includes('Invalid'));
    check(await page.locator('#appScreen').evaluate(el => el.classList.contains('hidden')), 'failed login does not open application');
    stage = 'registered login by keyboard'; check(await login(user, password) === 200, 'keyboard sign-in with registered identity');
    await page.waitForFunction(() => !document.getElementById('appScreen').classList.contains('hidden'));
    check(await page.locator('#password').inputValue() === '' && await page.locator('#username').inputValue() === '', 'login clears credential fields');
    check(await page.locator('.admin-only:visible').count() === 0, 'ordinary dashboard hides administrator controls');
    const me = await page.evaluate(async () => { const r=await fetch('/me'); return {status:r.status,body:await r.json()}; });
    check(me.status === 200 && me.body.is_admin === false, 'registered authenticated identity readback');
    const samples = [
      {name:'browser Δοκιμή 世界 with spaces & plus+.txt',bytes:Buffer.from('Real browser synthetic upload\n')},
      {name:'empty space Δ.txt',bytes:Buffer.alloc(0)},
      {name:'exact boundary.bin',bytes:Buffer.alloc(fileLimit,0x53)}
    ];
    for (const [index,sample] of samples.entries()) {
      stage = 'upload sample ' + index;
      const upload = page.waitForResponse(r => new URL(r.url()).pathname === '/upload' && r.request().method() === 'POST');
      const input = {name:sample.name,mimeType:'application/octet-stream',buffer:sample.bytes};
      if (index === 0) {
        const chooser = page.waitForEvent('filechooser');
        await page.getByRole('button',{name:'Choose File',exact:true}).focus(); await page.keyboard.press('Enter');
        await (await chooser).setFiles(input);
      } else await page.locator('#fileInput').setInputFiles(input);
      check((await upload).status() === 200, 'real multipart upload sample '+index);
      await page.waitForFunction(() => document.getElementById('linkContainer').classList.contains('show'));
      const link = await page.locator('#downloadLink').inputValue(); const target = new URL(link);
      check(target.origin === base && target.pathname === '/download', 'owned signed download origin sample '+index);
      const downloadPage = await context.newPage(); const event = downloadPage.waitForEvent('download');
      await downloadPage.goto(link).catch(error => { if (!/Download is starting|ERR_ABORTED/.test(error.message)) throw error; });
      const download = await event; const actual = await fs.readFile(await download.path());
      check(download.suggestedFilename() === sample.name && actual.equals(sample.bytes), 'attachment filename and byte equality sample '+index);
      await downloadPage.close();
      const listed = await page.evaluate(async () => (await fetch('/files')).json());
      const row = listed.find(file => file.orig_name === sample.name);
      check(row && row.sha256_hex === crypto.createHash('sha256').update(sample.bytes).digest('hex') && row.size_bytes === sample.bytes.length, 'browser metadata and native SHA-256 sample '+index);
      await page.waitForFunction(name => document.getElementById('filesOutput').textContent.includes(name),sample.name);
      check(true, 'visible owned listing sample '+index);
      await page.getByRole('button',{name:'Copy Link',exact:true}).click();
      check(await page.evaluate(() => window.fixtureCopied === document.getElementById('downloadLink').value), 'copy control writes exact link to isolated test clipboard sample '+index);
    }
    stage = 'upload rejection and recovery';
    await page.locator('#fileInput').setInputFiles({name:'too large.bin',mimeType:'application/octet-stream',buffer:Buffer.alloc(fileLimit+1)});
    await page.waitForFunction(() => document.getElementById('uploadAlert').textContent.includes('Upload failed'));
    check(await page.locator('#linkContainer').evaluate(el => !el.classList.contains('show')), 'oversize rejection leaves no successful link');
    await context.setOffline(true);
    await page.locator('#fileInput').setInputFiles({name:'interrupted.txt',mimeType:'text/plain',buffer:Buffer.from('x')});
    await page.waitForFunction(() => document.getElementById('uploadAlert').textContent.includes('Upload failed'));
    await context.setOffline(false); check(true,'browser offline upload fails without fake completion');
    const retry = page.waitForResponse(r => new URL(r.url()).pathname === '/upload' && r.request().method() === 'POST');
    await page.locator('#fileInput').setInputFiles({name:'recovered.txt',mimeType:'text/plain',buffer:Buffer.from('recovered')});
    check((await retry).status() === 200,'browser upload recovers after connectivity returns');
    await page.waitForFunction(() => document.getElementById('linkContainer').classList.contains('show'));
    const ownRows=await page.evaluate(async () => (await fetch('/files')).json());
    stage = 'responsive and safe evidence';
    await page.setViewportSize({width:390,height:844});
    check(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth+2),'mobile layout contains horizontal overflow');
    await page.evaluate(() => { for (const el of document.querySelectorAll('input')) el.value=''; });
    if (process.env.SFD_BROWSER_EVIDENCE) {
      await fs.mkdir(process.env.SFD_BROWSER_EVIDENCE,{recursive:true});
      await page.screenshot({path:path.join(process.env.SFD_BROWSER_EVIDENCE,process.env.SFD_BROWSER_RUN+'-registered-mobile.png'),fullPage:true});
    }
    stage='expired or absent session'; await context.clearCookies(); await page.evaluate(() => loadFiles());
    await page.waitForFunction(() => document.getElementById('appScreen').classList.contains('hidden'));
    check(await page.locator('#downloadLink').inputValue() === '', 'unauthorized session resets protected UI and link');
    await page.setViewportSize({width:1280,height:900});
    stage='administrator dashboard'; check(await login(fixture.username,fixture.password) === 200,'synthetic environment administrator UI login');
    await page.waitForFunction(() => !document.getElementById('appScreen').classList.contains('hidden'));
    check(await page.locator('.admin-nav').isVisible(),'administrator dashboard controls visible');
    await page.waitForFunction(() => document.getElementById('metricsOutput').textContent.includes('Total Uploads'));
    check(true,'administrator actual metrics render');
    await page.evaluate(() => { for (const el of document.querySelectorAll('input')) el.value=''; });
    if(process.env.SFD_BROWSER_EVIDENCE) await page.screenshot({path:path.join(process.env.SFD_BROWSER_EVIDENCE,process.env.SFD_BROWSER_RUN+'-admin.png'),fullPage:true});
    stage='owned browser fixture cleanup';
    const allowedNames=new Set([...samples.map(x=>x.name),'recovered.txt']);
    for(const row of ownRows.filter(x=>x.orig_name!==samples[0].name)){
      assert.ok(allowedNames.has(row.orig_name) && /^[0-9a-f-]{36}$/.test(row.id));
      const response=await page.evaluate(async id=>fetch('/admin/files/'+id,{method:'DELETE'}).then(r=>r.status),row.id);
      assert.equal(response,204);
    }
    stage='logout'; await page.getByRole('button',{name:'Sign out',exact:true}).click();
    await page.waitForFunction(() => document.getElementById('appScreen').classList.contains('hidden'));
    check((await page.request.get(base+'/me')).status() === 401,'actual logout rejects protected session');
    check(pageErrors.length === 0,'no uncaught browser errors');
    await context.close();
    console.log('PASS: real authenticated Chromium registration, login, multipart upload, native hash, link, byte-equal attachment download and logout');
    console.log('BROWSER CHECKS: '+JSON.stringify({count:checks.length,checks,OS_clipboard:'unexecuted; isolated test clipboard only',OS_native_file_picker:'unexecuted; browser filechooser event exercised'}));
  } finally { await browser.close(); }
})().catch(error => { console.error('BROWSER DIAGNOSTIC: '+JSON.stringify({stage,kind:error.name,assertion:error.name==='AssertionError'?error.message:'runtime failure; values redacted'})); process.exitCode=1; });