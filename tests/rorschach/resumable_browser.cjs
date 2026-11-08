const assert = require('node:assert/strict');
const {createHash} = require('node:crypto');
const {createReadStream} = require('node:fs');
const {join} = require('node:path');
const {chromium} = require('../browser/node_modules/playwright-core');

async function main() {
  const root = process.env.R1_RUNTIME;
  const origin = 'https://sfd.fixture.test:' + process.env.R1_ORIGIN_PORT;
  const fixture = join(root, 'r2-browser.bin');
  const hash = async filename => {
    const digest = createHash('sha256');
    for await (const data of createReadStream(filename)) digest.update(data);
    return digest.digest('hex');
  };
  const browser = await chromium.launch({
    env: {...process.env, HOME: join(root, 'browser-home')},
    args: ['--host-resolver-rules=MAP sfd.fixture.test 127.0.0.1', '--no-proxy-server'],
  });
  try {
    const page = await browser.newPage({viewport:{width:390,height:844}, acceptDownloads:true});
    assert.equal((await page.goto(origin)).status(), 200);
    await page.locator('#username').fill('fixture');
    await page.locator('#password').fill(process.env.SFD_R1_FIXTURE_USER_PASSWORD);
    await page.getByRole('button', {name:'Sign In',exact:true}).click();
    await page.locator('#resumablePanel').waitFor({state:'visible'});
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), true);
    await page.locator('#resumableFile').setInputFiles(fixture);
    let interrupted = false;
    await page.route('**/uploads/resumable/*', async route => {
      if (!interrupted && route.request().method() === 'PATCH' &&
          route.request().headers()['upload-offset'] === String(32 * 1024 * 1024)) {
        interrupted = true;
        await route.abort('connectionreset');
      } else await route.continue();
    });
    await page.locator('#resumableStart').click();
    await page.getByText('Connection interrupted. Resume to check committed progress.', {exact:true}).waitFor({timeout:90000});
    assert.equal(interrupted, true);
    await page.reload();
    await page.locator('#resumableSaved option').filter({hasText:'r2-browser.bin'}).waitFor({state:'attached'});
    const saved = await page.locator('#resumableSaved option').filter({hasText:'r2-browser.bin'}).getAttribute('value');
    await page.locator('#resumableSaved').selectOption(saved);
    await page.locator('#resumableFile').setInputFiles(join(root, 'different-file', 'r2-browser.bin'));
    await page.locator('#resumableStart').click();
    await page.getByText('The selected file differs from the original file. Reselect the original to resume.', {exact:true}).waitFor();
    await page.locator('#resumableFile').setInputFiles(fixture);
    await page.locator('#resumableStart').click();
    await page.getByText('Upload verified. Create a link when you are ready to share.', {exact:true}).waitFor({timeout:90000});
    await page.locator('#resumableLink').click();
    await page.locator('#resumableLinkBox').waitFor({state:'visible'});
    const download = page.waitForEvent('download');
    await page.locator('#resumableLinkValue').evaluate(input => window.location.assign(input.value));
    const savedFile = await download;
    assert.equal(await hash(await savedFile.path()), await hash(fixture));
    await page.unroute('**/uploads/resumable/*');
    let partArrived, releasePart;
    const arrival = new Promise(resolve => { partArrived = resolve; });
    const hold = new Promise(resolve => { releasePart = resolve; });
    await page.route('**/uploads/resumable/*', async route => {
      if (route.request().method() !== 'PATCH') {
        await route.continue();
        return;
      }
      partArrived();
      await hold;
      await route.abort('aborted').catch(() => {});
    });
    await page.locator('#resumableSaved').selectOption('');
    await page.locator('#resumableFile').setInputFiles({
      name:'browser-cancel.bin', mimeType:'application/octet-stream', buffer:Buffer.from('owned cancellation fixture')
    });
    await page.locator('#resumableStart').click();
    try {
      await Promise.race([arrival, new Promise((_, reject) => setTimeout(() => reject(new Error('part did not start')), 30000).unref())]);
      await page.locator('#resumablePause').click();
      await page.getByText('Paused. Resume to check the server’s committed progress.', {exact:true}).waitFor();
    } finally {
      releasePart();
    }
    await page.locator('#resumableCancel').click();
    await page.getByText('Cancellation accepted. Storage cleanup continues on the server.', {exact:true}).waitFor();
    console.log('PASS: strict HTTPS Chromium upload, interrupted second part, reload/reselect, resume and byte-exact download');
    console.log('PASS: Chromium pause and real cancellation request');
  } finally {
    await browser.close();
  }
}

main().catch(error => {
  console.error('Resumable browser check failed:', error.message.split('\n')[0]);
  process.exitCode = 1;
});
