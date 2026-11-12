const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const {join} = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');
const id = '11111111-1111-4111-8111-111111111111';
const row = {id,file_id:id,file_name:'<img src=x onerror=private()>',state:'active',
  created_at:'2026-11-11T00:00:00Z',expires_at:'2026-11-12T00:00:00Z'};
function harness(fetcher) {
  const nodes = new Map();
  const element = () => ({children:[],value:'',textContent:'',hidden:true,disabled:false,dataset:{},listeners:{},
    append(...items){this.children.push(...items)},replaceChildren(...items){this.children=items;this.value=''},
    addEventListener(name, listener){this.listeners[name]=listener},showModal(){this.open=true},close(){this.open=false},focus(){},select(){}});
  const node = name => {if(!nodes.has(name)) nodes.set(name,element()); return nodes.get(name)};
  const context = vm.createContext({document:{getElementById:node,createElement:element},
    Option:function(text,value){return {text,value}},sessionFetch:fetcher,URL,URLSearchParams,
    location:{origin:'https://fixture.test'},navigator:{clipboard:{writeText:async()=>{}}},
    isLoggedIn:false,isAdmin:false,sessionGeneration:1,setTimeout(){return 1},clearTimeout(){}});
  vm.runInContext(readFileSync(join(__dirname,'../../web/static/managed-links.js'),'utf8'),context);
  context.SFDManagedLinks.configure({managed_links_enabled:true});
  context.isLoggedIn=true;
  return {context,node,links:context.SFDManagedLinks};
}
function response(rows, cursor='') {return {ok:true,status:200,headers:{get:()=>cursor},json:async()=>rows};}

test('disabled configuration never requests or displays managed metadata',async()=>{
  let calls=0;const h=harness(async()=>{calls++;return response([row])});
  h.links.configure({managed_links_enabled:false});await h.links.refresh();
  assert.equal(calls,0);assert.equal(h.node('managedLinksPanel').hidden,true);
});
test('managed filenames are text nodes and listing errors remove stale active cards',async()=>{
  let fail=false;const h=harness(async()=>fail?{ok:false,status:503}:response([row]));
  await h.links.refresh();
  const heading=h.node('managedList').children[0].children[0];
  assert.equal(heading.textContent,row.file_name);assert.equal(heading.innerHTML,undefined);
  fail=true;await h.links.refresh();
  assert.equal(h.node('managedList').children.length,0);
  assert.match(h.node('managedStatus').textContent,/Could not verify/);
});
test('parsed metadata from an old session cannot render after reset and login',async()=>{
  let finish, reading;const started=new Promise(resolve=>reading=resolve),body=new Promise(resolve=>finish=resolve);
  const h=harness(async()=>({ok:true,headers:{get:()=>''},json(){reading();return body}}));
  const pending=h.links.refresh();await started;
  h.links.reset();h.context.sessionGeneration++;h.context.isLoggedIn=true;
  finish([row]);await pending;
  assert.equal(h.node('managedList').children.length,0);assert.equal(h.node('managedLinksPanel').hidden,true);
  assert.equal(h.node('managedURL').value,'');
});
test('a later filter request wins over a delayed parsed previous filter',async()=>{
  let finish,reading,calls=0;const started=new Promise(resolve=>reading=resolve),body=new Promise(resolve=>finish=resolve);
  const h=harness(async()=>++calls===1?{ok:true,headers:{get:()=>''},json(){reading();return body}}:response([]));
  const old=h.links.refresh();await started;await h.links.refresh();finish([row]);await old;
  assert.equal(h.node('managedList').children.length,0);assert.match(h.node('managedStatus').textContent,/No sharing links/);
});
test('malformed metadata, cursor and private errors never render into the panel',async()=>{
  for (const fetcher of [async()=>response([{...row,state:'PRIVATE'}]),async()=>response([row],'PRIVATE'),
    async()=>({ok:true,headers:{get:()=>''},json:async()=>{throw Error('PRIVATE')}})]) {
    const h=harness(fetcher);await h.links.refresh();assert.equal(h.node('managedList').children.length,0);
    assert.doesNotMatch(h.node('managedStatus').textContent,/PRIVATE/);
  }
});
test('reset removes issued bearer, choices, dialog target and list',async()=>{
  const h=harness(async()=>response([row]));await h.links.refresh();
  h.node('managedURL').value='synthetic-private-bearer';h.node('managedConfirm').open=true;
  h.links.reset();assert.equal(h.node('managedURL').value,'');assert.equal(h.node('managedConfirm').open,false);
  assert.equal(h.node('managedConfirmName').textContent,'');assert.equal(h.node('managedList').children.length,0);
  assert.equal(h.node('managedFile').children.length,1);
});
test('delayed configuration populates the current account files without retaining an earlier account list',async()=>{
  const h=harness(async()=>response([]));
  h.links.configure({managed_links_enabled:false});
  const files=[{id,orig_name:'current-account.txt',status:'hashed'}];
  h.links.files(files);assert.equal(h.node('managedFile').children.length,1);
  h.links.configure({managed_links_enabled:true},files);
  assert.equal(h.node('managedFile').children.length,2);
  assert.equal(h.node('managedFile').children[1].text,'current-account.txt');
  h.links.reset();h.context.sessionGeneration++;
  h.links.configure({managed_links_enabled:true},[]);
  assert.equal(h.node('managedFile').children.length,1);
});
