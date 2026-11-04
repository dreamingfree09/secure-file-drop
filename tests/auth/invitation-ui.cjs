const assert=require('node:assert/strict');
const crypto=require('node:crypto');

// Runs only inside the existing owned synthetic HTTPS browser fixture. It does
// not send real provider mail or access the Owner's browser/session/clipboard.
async function verifyInvitationUI(page,origin) {
  const checks=[],screenshots=[];
  function check(value,name){assert.ok(value,name);checks.push(name)}
  await page.locator('#deliveryFile').selectOption({index:1});
  await page.locator('#deliveryRecipient').fill('recipient@example.test');
  await page.locator('#deliveryDescription').fill('A small synthetic file for a protected delivery.');
  await page.locator('#deliveryRecipient').focus();
  check(await page.locator('#deliveryRecipient').evaluate(e=>getComputedStyle(e).outlineStyle==='solid'&&getComputedStyle(e).outlineWidth==='3px'),'visible keyboard focus indicator');
  for(const width of [1280,375,320]) {
    await page.setViewportSize({width,height:1000});
    const layout=await page.locator('#deliveryPanel').evaluate(panel=>{
      const controls=[...panel.querySelectorAll('select,textarea,input:not([readonly]),button[type=submit]')];
      return {inside:controls.every(e=>{const r=e.getBoundingClientRect();return r.left>=0&&r.right<=innerWidth&&r.width>0}),
        minTarget:controls.every(e=>e.getBoundingClientRect().height>=44),
        consistentFont:controls.every(e=>getComputedStyle(e).fontFamily===getComputedStyle(panel).fontFamily),
        labels:[...panel.querySelectorAll('select,textarea,input:not([readonly])')].every(e=>!!panel.querySelector('label[for="'+e.id+'"]')),
        columns:getComputedStyle(panel.querySelector('form')).gridTemplateColumns.split(' ').length};
    });
    check(layout.inside,'invitation controls contained at '+width+'px');
    check(layout.minTarget&&layout.consistentFont,'consistent typography and touch targets at '+width+'px');
    check(layout.labels&&layout.columns===(width>640?2:1),'labelled responsive invitation grid at '+width+'px');
    if(width!==320) {
      const png=await page.locator('#deliveryPanel').screenshot({animations:'disabled'});
      screenshots.push({name:'invitation-'+width+'.png',sha256:crypto.createHash('sha256').update(png).digest('hex'),base64:png.toString('base64')});
    }
  }
  await page.setViewportSize({width:1280,height:900});
  await page.route(origin+'/invitations',route=>route.fulfill({status:503,contentType:'application/json',body:JSON.stringify({error:'email_test_recipient_restricted',message:'PRIVATE synthetic provider payload'})}));
  try {
    await page.locator('#sendDeliveryButton').click();
    await page.waitForFunction(()=>document.getElementById('deliveryAlert').textContent.includes('test sender can email only'));
    check(!await page.locator('#deliveryCodeBox').isVisible()&&await page.locator('#deliveryCode').inputValue()==='','rejected mail displays no access code');
    check(!(await page.locator('#deliveryAlert').textContent()).includes('PRIVATE'),'provider response omitted from rendered error');
    check(await page.locator('#sendDeliveryButton').isEnabled(),'delivery failure restores send button');
    check(await page.locator('#deliveryAlert').getAttribute('aria-live')==='polite','accessible invitation status');
  } finally {await page.unroute(origin+'/invitations');}
  return {count:checks.length,checks,screenshots,scope:'isolated real Chromium form rendering and mocked provider-error UI; real SMTP delivery is independently tested by backend fixture'};
}
module.exports={verifyInvitationUI};
