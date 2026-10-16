const assert = require('node:assert/strict');
const fs = require('node:fs/promises');
const crypto = require('node:crypto');
const { chromium } = require('playwright-core');

(async () => {
  const base = process.argv[2];
  const parsed = new URL(base);
  assert.equal(parsed.protocol, 'http:');
  assert.equal(parsed.hostname, '127.0.0.1');
  const browser = await chromium.launch({ headless: true, chromiumSandbox: true });
  try {
    const context = await browser.newContext({ acceptDownloads: true });
    const page = await context.newPage();
    const failures = [];
    page.on('pageerror', error => failures.push(error.message));
    const user = 'browser_' + crypto.randomBytes(6).toString('hex');
    const password = 'SyntheticBrowser123';
    const bytes = Buffer.from('Real authenticated Chromium upload and native hash download\n');
    await page.goto(base);
    await page.evaluate(() => showRegisterForm());
    await page.locator('#regEmail').fill(user + '@example.test');
    await page.locator('#regUsername').fill(user);
    await page.locator('#regPassword').fill(password);
    await page.locator('#regPasswordConfirm').fill(password);
    const registered = page.waitForResponse(r => r.url() === base + '/register' && r.request().method() === 'POST');
    await page.evaluate(() => register());
    assert.equal((await registered).status(), 201);
    for (const id of ['regEmail', 'regUsername', 'regPassword', 'regPasswordConfirm']) {
      assert.equal(await page.locator('#' + id).inputValue(), '');
    }
    await page.waitForFunction(() => !document.getElementById('loginForm').classList.contains('hidden'));
    await page.locator('#username').fill(user);
    await page.locator('#password').fill(password);
    await page.evaluate(() => login());
    await page.waitForFunction(() => !document.getElementById('appScreen').classList.contains('hidden'));
    assert.equal(await page.locator('#password').inputValue(), '');
    assert.equal(await page.locator('#username').inputValue(), '');
    await page.locator('#fileInput').setInputFiles({ name: 'browser-fixture.txt', mimeType: 'text/plain', buffer: bytes });
    await page.waitForFunction(() => document.getElementById('linkContainer').classList.contains('show'));
    const link = await page.locator('#downloadLink').inputValue();
    assert.ok(link.startsWith(base + '/download?'));
    const downloadPage = await context.newPage();
    const downloadEvent = downloadPage.waitForEvent('download');
    await downloadPage.goto(link).catch(error => {
      // Chromium reports an aborted navigation when it starts an attachment download.
      if (!error.message.includes('Download is starting') && !error.message.includes('ERR_ABORTED')) throw error;
    });
    const download = await downloadEvent;
    assert.equal(download.suggestedFilename(), 'browser-fixture.txt');
    assert.deepEqual(await fs.readFile(await download.path()), bytes);
    await page.evaluate(() => logout());
    await page.waitForFunction(() => document.getElementById('appScreen').classList.contains('hidden'));
    assert.equal(await page.locator('#downloadLink').inputValue(), '');
    assert.equal(await page.locator('#regPassword').inputValue(), '');
    assert.deepEqual(failures, []);
    await context.close();
    console.log('PASS: real authenticated Chromium registration, login, multipart upload, native hash, link, byte-equal attachment download and logout');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
