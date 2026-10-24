const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const { test } = require('node:test');

function harness(fetch) {
  const nodes = new Map();
  function node(id) {
    if (!nodes.has(id)) {
      const classes = new Set();
      nodes.set(id, { value: '', textContent: '', innerHTML: '', style: {}, files: [],
        addEventListener() {},
        classList: { add(x) { classes.add(x); }, remove(x) { classes.delete(x); },
          toggle(x, value) { if (value) classes.add(x); else classes.delete(x); },
          contains(x) { return classes.has(x); } } });
    }
    const element=nodes.get(id);
    if (!element.domContent) {let content='';Object.defineProperties(element,{innerHTML:{get(){return content},set(value){content=value}},textContent:{get(){return content},set(value){content=value}}});element.domContent=true;}
    return nodes.get(id);
  }
  const context = vm.createContext({
    document: { readyState: 'loading', addEventListener() {}, getElementById: node, querySelectorAll: () => [] },
    fetch, console, DOMException, AbortController, clearTimeout, TextEncoder, setTimeout() {}
  });
  const html = fs.readFileSync(path.join(__dirname, '../../web/static/index.html'), 'utf8');
  vm.runInContext(html.split('<script>')[1].split('</script>')[0], context);
  return { context, node };
}

test('logout removes previous-account metadata and selected file', async () => {
  const h = harness(async () => ({ ok: true, status: 204 }));
  h.node('fileName').textContent = 'private-fixture.txt';
  h.node('downloadLink').value = 'synthetic-bearer-link';
  h.node('filesOutput').innerHTML = 'previous account file list';
  h.node('filesOutput').textContent = 'previous account file list';
  h.node('fileInput').value = 'fixture';
  h.node('fileInfo').classList.add('show');
  vm.runInContext('isLoggedIn=true; isAdmin=true; selectedFile={name:"fixture"}; sessionGeneration=3;', h.context);
  await vm.runInContext('logout()', h.context);
  assert.equal(h.node('fileName').textContent, '');
  assert.equal(h.node('downloadLink').value, '');
  assert.equal(h.node('fileInput').value, '');
  assert.equal(h.node('filesOutput').textContent, '');
  assert.equal(h.node('fileInfo').classList.contains('show'), false);
  assert.equal(h.node('appScreen').classList.contains('hidden'), true);
  assert.equal(vm.runInContext('selectedFile === null && !isLoggedIn && !isAdmin', h.context), true);
});

test('a delayed file-list response cannot render into a later session', async () => {
  let deliver;
  const delayed = new Promise(resolve => { deliver = resolve; });
  let signal;
  const h = harness(async (url, options) => { if (url === '/admin/files') { signal = options.signal; return delayed; } return {ok: true, status: 204}; });
  vm.runInContext('isLoggedIn=true; isAdmin=false; sessionGeneration=7;', h.context);
  const pending = vm.runInContext('loadFiles()', h.context);
  await vm.runInContext('logout()', h.context);
  assert.equal(signal.aborted, true);
  // A different account logs in before the old response completes.
  vm.runInContext('isLoggedIn=true; sessionGeneration++;', h.context);
  deliver({ ok: true, async json() { return [{ orig_name: 'previous-private-file' }]; } });
  await pending;
  assert.equal(h.node('filesOutput').innerHTML, '');
  assert.equal(h.node('filesOutput').textContent, '');
});

test('alerts escape untrusted server text', () => {
  const h = harness(async () => { throw new Error('network must not be used'); });
  vm.runInContext('showAlert(document.getElementById("registerAlert"), "<img src=x onerror=alert(1)>", "error")', h.context);
  assert.match(h.node('registerAlert').innerHTML, /&lt;img/);
  assert.doesNotMatch(h.node('registerAlert').innerHTML, /<img/);
});

test('successful login clears login and registration credentials', async () => {
  const h = harness(async url => ({ok: true, json: async () => url === '/me' ? {status:'ok',username:'fixture',is_admin:false} : []}));
  h.node('username').value = 'fixture';
  h.node('password').value = 'SyntheticPass123';
  h.node('regPassword').value = 'OldRegistration123';
  h.node('regPasswordConfirm').value = 'OldRegistration123';
  await vm.runInContext('login()', h.context);
  for (const id of ['username','password','regPassword','regPasswordConfirm']) assert.equal(h.node(id).value, '');
  assert.equal(vm.runInContext('isLoggedIn && !isAdmin',h.context), true);
});

test('successful registration clears all submitted fields', async () => {
  const h = harness(async () => ({ok: true, json: async () => ({id: 'synthetic'})}));
  h.node('regEmail').value = 'fixture@example.test';
  h.node('regUsername').value = 'fixture';
  h.node('regPassword').value = 'SyntheticPass123';
  h.node('regPasswordConfirm').value = 'SyntheticPass123';
  await vm.runInContext('register()',h.context);
  for (const id of ['regEmail','regUsername','regPassword','regPasswordConfirm']) assert.equal(h.node(id).value, '');
});


test('protected HTTP401 clears expired-session metadata, links and UI', async () => {
  const h = harness(async () => ({ok:false,status:401}));
  vm.runInContext('isLoggedIn=true; isAdmin=false; selectedFile={name:"synthetic"};',h.context);
  h.node('downloadLink').value='synthetic-expired-link';h.node('filesOutput').textContent='previous synthetic file';
  await vm.runInContext('loadFiles()',h.context);
  assert.equal(vm.runInContext('!isLoggedIn && !isAdmin && selectedFile===null',h.context),true);
  assert.equal(h.node('downloadLink').value,'');assert.equal(h.node('filesOutput').textContent,'');
  assert.equal(h.node('appScreen').classList.contains('hidden'),true);
});

test('an old HTTP401 cannot sign out a newer authenticated account', async () => {
  let deliver;const delayed=new Promise(resolve=>{deliver=resolve;});
  const h=harness(async url=>url==='/admin/files'?delayed:{ok:true,status:204});
  vm.runInContext('isLoggedIn=true; isAdmin=false; sessionGeneration=7;',h.context);
  const pending=vm.runInContext('loadFiles()',h.context);await vm.runInContext('logout()',h.context);
  vm.runInContext('isLoggedIn=true; isAdmin=false; sessionGeneration++;',h.context);
  h.node('filesOutput').textContent='new synthetic account';deliver({ok:false,status:401});await pending;
  assert.equal(vm.runInContext('isLoggedIn',h.context),true);assert.equal(h.node('filesOutput').textContent,'new synthetic account');
});

test('forbidden HTTP403 does not discard a valid ordinary-user session', async () => {
  const h=harness(async()=>({ok:false,status:403}));vm.runInContext('isLoggedIn=true; isAdmin=false;',h.context);
  const response=await vm.runInContext("sessionFetch('/admin/files')",h.context);
  assert.equal(response.status,403);assert.equal(vm.runInContext('isLoggedIn && !isAdmin',h.context),true);
});
