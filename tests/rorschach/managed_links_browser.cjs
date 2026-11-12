const assert = require('node:assert/strict');
const {createHash} = require('node:crypto');
const {readFileSync, writeFileSync} = require('node:fs');
const {join} = require('node:path');
const {chromium} = require('../browser/node_modules/playwright-core');

async function main() {
  const root = process.env.R1_RUNTIME, phase = process.argv[2];
  const origin = 'https://sfd.fixture.test:' + process.env.R1_ORIGIN_PORT;
  const browser = await chromium.launch({env:{...process.env, HOME:join(root, 'managed-browser-home')},
    args:['--host-resolver-rules=MAP sfd.fixture.test 127.0.0.1', '--no-proxy-server']});
  const context = await browser.newContext({viewport:{width:1280,height:900}, acceptDownloads:true});
  await context.grantPermissions(['clipboard-read', 'clipboard-write'], {origin});
  const page = await context.newPage();
  const errors = [];
  page.on('pageerror', error => errors.push(error.name));
  const login = async username => {
    await page.waitForFunction(() => !logoutPending);
    await page.locator('#username').fill(username);
    await page.locator('#password').fill(process.env.SFD_R1_FIXTURE_USER_PASSWORD);
    await page.getByRole('button', {name:'Sign In',exact:true}).click();
    await page.locator('#logoutBtn').waitFor({state:'visible'});
    await page.waitForFunction(() => document.querySelector('#password').value === '');
  };
  const ready = async () => {
    await page.locator('#managedLinksPanel').waitFor({state:'visible'});
    await page.waitForFunction(() => !document.querySelector('#managedRefresh').disabled);
  };
  const refresh = async () => {
    const receipt = page.waitForResponse(r => new URL(r.url()).pathname === '/managed-links');
    await page.locator('#managedRefresh').click(); await receipt; await ready();
  };
  const api = async (method, path, value) => {
    const result = await page.evaluate(async ({method, path, value}) => {
      const response = await fetch(path, {method, headers:value ? {'Content-Type':'application/json'} : {},
        body:value ? JSON.stringify(value) : undefined});
      return {status:response.status, body:await response.text()};
    }, {method, path, value});
    return {status:() => result.status, text:async () => result.body, json:async () => JSON.parse(result.body)};
  };
  const range = async url => {
    const status = await page.evaluate(async url => (await fetch(url,{headers:{Range:'bytes=0-7'}})).status,url);
    return {status:() => status};
  };
  try {
    assert.equal((await page.goto(origin)).status(), 200);
    await login('fixture');
    if (phase === 'disabled') {
      assert.equal(await page.locator('#managedLinksPanel').isVisible(), false);
      assert.equal(await page.locator('#managedURL').inputValue(), '');
      assert.equal((await api('GET','/config')).status(), 200);
      console.log('PASS: managed UI remains hidden on default schema13');
      return;
    }
    await ready();
    if (phase === 'history') {
      const select = page.locator('#managedFile option').filter({hasText:'rorschach-fixture.txt'});
      await select.waitFor({state:'attached'});
      await page.locator('#managedFile').selectOption(await select.getAttribute('value')); await ready();
      await page.locator('.managed-card').first().waitFor();
      assert.equal(await page.locator('.managed-card').count(),50);
      await page.locator('#managedMore').click(); await ready();
      await page.waitForFunction(() => document.querySelectorAll('.managed-card').length > 50);
      assert.equal(await page.locator('.managed-state-expired').count(),54);
      assert.equal(await page.locator('#managedMore').isVisible(),false);
      console.log('PASS: real Chromium server-expired statuses and all older links across cursor pages');
      return;
    }
    if (phase === 'restart') {
      const state = JSON.parse(readFileSync(join(root,'managed-browser-receipt.json'),'utf8'));
      await page.locator('#managedFile').selectOption(state.file);
      await ready();
      const card = page.locator('[data-link-id="' + state.revoked + '"]');
      await card.locator('.managed-state-revoked').waitFor();
      assert.equal((await api('GET', '/download?' + new URL(state.url).searchParams)).status(), 410);
      assert.equal((await range(state.url.replace('/share?', '/download?'))).status(), 410);
      assert.equal((await api('GET',state.other.replace('/share?', '/download?'))).status(), 200);
      console.log('PASS: real Chromium reauthentication after backend restart preserves revocation and other-link access');
      return;
    }
    // Use the real existing upload control: no synthetic browser-side API substitute.
    const payload = Buffer.from('Managed browser upload and native SHA-256 fixture.\n'.repeat(97));
    const filename = 'managed-browser-owned.txt';
    await page.locator('#fileInput').setInputFiles({name:filename, mimeType:'text/plain', buffer:payload});
    await page.locator('#linkContainer.show').waitFor();
    await page.locator('#managedFile option').filter({hasText:filename}).waitFor({state:'attached'});
    const file = await page.locator('#managedFile option').filter({hasText:filename}).getAttribute('value');
    await page.locator('#managedFile').selectOption(file); await ready();
    const issued = page.waitForResponse(r => new URL(r.url()).pathname === '/links' && r.request().method() === 'POST');
    await page.locator('#managedCreate').click();
    const issueResponse = await issued; assert.equal(issueResponse.status(),200);
    const link = await issueResponse.json();
    await page.locator('#managedIssued').waitFor({state:'visible'}); await ready();
    const url = await page.locator('#managedURL').inputValue();
    assert.equal(url, link.url); assert.equal(new URL(url).pathname, '/share');
    await page.locator('#managedCopy').click();
    assert.equal(await page.evaluate(() => navigator.clipboard.readText()), url);
    const preview = await browser.newPage({acceptDownloads:true});
    try {
      const landing = await preview.goto(url);
      assert.equal(landing.status(),200);
      assert.equal((await landing.body()).includes(payload), false);
      assert.equal((await landing.text()).includes(filename),false);
      const download = preview.waitForEvent('download');
      await preview.getByRole('button',{name:'Download file',exact:true}).click();
      const actual = readFileSync(await (await download).path());
      assert.equal(createHash('sha256').update(actual).digest('hex'),createHash('sha256').update(payload).digest('hex'));
      assert.deepEqual(actual,payload);
    } finally { await preview.close(); }
    const second = await api('POST','/links',{id:file,ttl_seconds:3600,share_page:true});
    assert.equal(second.status(),200); const other = await second.json();
    const listing = await api('GET','/managed-links?file_id='+file);
    const listed = await listing.text(); assert.equal(listed.includes(new URL(url).searchParams.get('token')),false);
    assert.equal(listed.includes('token_sha256'),false);
    const card = page.locator('[data-link-id="' + link.link_id + '"]');
    await card.getByRole('button',{name:'Revoke link',exact:true}).click();
    assert.equal(await page.locator('#managedConfirm').evaluate(node => node.open),true);
    await page.locator('#managedConfirmCancel').click();
    assert.equal((await api('GET',url.replace('/share?', '/download?'))).status(),200);
    await card.getByRole('button',{name:'Revoke link',exact:true}).click();
    await page.locator('#managedConfirmYes').click();
    await card.locator('.managed-state-revoked').waitFor();
    assert.equal(await page.locator('#managedURL').inputValue(),'');
    const raw = url.replace('/share?', '/download?');
    assert.equal((await api('GET',raw)).status(),410);
    assert.equal((await range(raw)).status(),410);
    assert.equal((await api('GET',other.url.replace('/share?', '/download?'))).status(),200);
    assert.equal((await range(other.url.replace('/share?', '/download?'))).status(),206);
    writeFileSync(join(root,'managed-browser-receipt.json'),JSON.stringify({file,revoked:link.link_id,url,other:other.url}),{mode:0o600});
    await page.screenshot({path:join(root,'managed-desktop.png'),fullPage:true,mask:[page.locator('#linkContainer'),page.locator('#managedIssued')]});
    await page.setViewportSize({width:390,height:844});
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth),true);
    await page.screenshot({path:join(root,'managed-mobile.png'),fullPage:true,mask:[page.locator('#linkContainer'),page.locator('#managedIssued')]});
    // Failed network reads cannot continue to display stale 'active' status.
    await page.route('**/managed-links?*', route => route.abort('connectionreset'));
    await page.locator('#managedRefresh').click();
    await page.getByText('Could not verify your links. Refresh to try again.',{exact:true}).waitFor();
    assert.equal(await page.locator('.managed-card').count(),0);
    await page.unroute('**/managed-links?*'); await refresh();
    // A failed revocation is never reported as successful.
    const otherCard = page.locator('[data-link-id="' + other.link_id + '"]');
    await page.route('**/managed-links/'+other.link_id, route => route.fulfill({status:503,body:'private diagnostic must not render'}));
    await otherCard.getByRole('button',{name:'Revoke link',exact:true}).click();
    await page.locator('#managedConfirmYes').click();
    await page.getByText('Revocation could not be confirmed. Refresh to check the server before sharing again.',{exact:true}).waitFor();
    assert.equal((await page.locator('#managedStatus').textContent()).includes('private diagnostic'),false);
    await page.unroute('**/managed-links/'+other.link_id); await refresh();
    await otherCard.getByText('Active',{exact:true}).waitFor();
    // Real account switch: both metadata and just-issued bearer disappear.
    await page.locator('#logoutBtn').click();
    await page.locator('#loginScreen').waitFor({state:'visible'});
    assert.equal(await page.locator('#managedList').textContent(),'');
    assert.equal(await page.locator('#managedURL').inputValue(),'');
    assert.equal((await api('GET','/managed-links')).status(),401);
    await login('managed-other'); await ready();
    assert.equal(await page.locator('.managed-card').count(),0);
    assert.equal((await api('DELETE','/managed-links/'+link.link_id)).status(),404);
    assert.deepEqual(await (await api('GET','/managed-links?file_id='+file)).json(),[]);
    assert.equal((await page.locator('#managedFile').textContent()).includes(filename),false);
    // Expired authentication clears the actual account-bound panel.
    await api('POST','/logout');
    await page.locator('#managedRefresh').click();
    await page.locator('#loginScreen').waitFor({state:'visible'});
    assert.equal(await page.locator('#managedLinksPanel').isVisible(),false);
    assert.equal(await page.locator('#managedList').textContent(),'');
    assert.deepEqual(errors,[]);
    console.log('PASS: Chromium real login/upload/native-hashed download, clipboard, generic preview, confirmed revoke/full/Range denial, other-link survival');
    console.log('PASS: Chromium mobile/desktop, failed listing/revocation, logout/account isolation and expired-session clearing');
  } finally { await context.close(); await browser.close(); }
}
main().catch(error => { console.error('Managed browser acceptance failed:', error.message.split('\n')[0]); process.exitCode=1; });
