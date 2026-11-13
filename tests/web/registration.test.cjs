const assert=require('node:assert/strict');
const {readFileSync}=require('node:fs');
const {join}=require('node:path');
const vm=require('node:vm');
const {test}=require('node:test');
const id='11111111-1111-4111-8111-111111111111';
const row={id,state:'active',created_at:'2026-11-13T00:00:00Z',expires_at:'2026-11-14T00:00:00Z'};
function nodes(){const all=new Map();const create=()=>({children:[],value:'',textContent:'',hidden:true,dataset:{},listeners:{},
  append(...items){this.children.push(...items)},replaceChildren(...items){this.children=items},addEventListener(name,fn){this.listeners[name]=fn},
  showModal(){this.open=true},close(){this.open=false},focus(){},select(){},reset(){for(const node of all.values())node.value=''}});
  return {get(id){if(!all.has(id))all.set(id,create());return all.get(id)},create};}
const response=(rows)=>({ok:true,status:200,headers:{get:()=>''},json:async()=>rows});
function admin(fetcher){
  const n=nodes();const context=vm.createContext({document:{getElementById:id=>n.get(id),createElement:n.create},
    sessionFetch:fetcher,isLoggedIn:false,isAdmin:true,sessionGeneration:1,URL,URLSearchParams,location:{origin:'https://fixture.test'},
    navigator:{clipboard:{writeText:async()=>{}}},setTimeout(){return 1},clearTimeout(){}});
  vm.runInContext(readFileSync(join(__dirname,'../../web/static/registration-admin.js'),'utf8'),context);
  const api=context.SFDRegistrationAdmin;api.configure({registration_invitations_enabled:true});api.activate({environment_admin:true});context.isLoggedIn=true;
  return {context,node:id=>n.get(id),api};
}
test('registration controls require explicit feature flag and environment issuer identity',async()=>{
  let calls=0;const h=admin(async()=>{calls++;return response([row])});
  h.api.activate({environment_admin:false,is_admin:true});await h.api.refresh();assert.equal(calls,0);
  h.api.configure({registration_invitations_enabled:false});await h.api.refresh();assert.equal(calls,0);assert.equal(h.node('registrationAdminPanel').hidden,true);
});
test('late parsed invitation metadata cannot render into a changed session',async()=>{
  let finish,reading;const body=new Promise(resolve=>finish=resolve),started=new Promise(resolve=>reading=resolve);
  const h=admin(async()=>({ok:true,headers:{get:()=>''},json(){reading();return body}}));const work=h.api.refresh();await started;
  h.api.reset();h.context.sessionGeneration++;h.context.isLoggedIn=true;finish([row]);await work;
  assert.equal(h.node('registrationAdminList').children.length,0);assert.equal(h.node('registrationAdminPanel').hidden,true);
});
test('failed metadata refresh removes stale administrative cards and suppresses diagnostics',async()=>{
  let fail=false;const h=admin(async()=>fail?{ok:false,status:503}:response([row]));await h.api.refresh();
  assert.equal(h.node('registrationAdminList').children.length,1);fail=true;await h.api.refresh();
  assert.equal(h.node('registrationAdminList').children.length,0);assert.match(h.node('registrationAdminStatus').textContent,/Could not verify/);
});
test('logout clears the one-time invitation, list and pending dialog',async()=>{
  const h=admin(async()=>response([row]));await h.api.refresh();h.node('registrationURL').value='private synthetic capability';h.node('registrationRevokeDialog').open=true;
  h.api.reset();assert.equal(h.node('registrationURL').value,'');assert.equal(h.node('registrationAdminList').children.length,0);assert.equal(h.node('registrationRevokeDialog').open,false);
});
test('a late successful issuance cannot disclose its invitation in the next account',async()=>{
  let finish;const body=new Promise(resolve=>finish=resolve);
  const h=admin(async(path,options)=>options?.method==='POST'?body:response([]));h.node('registrationTTL').value='3600';
  const work=h.node('registrationIssue').listeners.click();
  h.api.reset();h.context.sessionGeneration++;h.context.isLoggedIn=true;
  finish({status:201,json:async()=>({id,url:'https://fixture.test/join#token='+'a'.repeat(64)})});await work;
  await new Promise(resolve=>setImmediate(resolve));assert.equal(h.node('registrationURL').value,'');assert.equal(h.node('registrationReceipt').hidden,true);
});
function recipient(fetcher,fragment='token='+'a'.repeat(64)){
  const n=nodes(),events={},history=[];
  const context=vm.createContext({document:{getElementById:id=>n.get(id)},location:{hash:'#'+fragment,pathname:'/join'},
    history:{replaceState(...args){history.push(args)}},URLSearchParams,AbortController,fetch:fetcher,
    setTimeout(){return 1},clearTimeout(){},addEventListener(name,fn){events[name]=fn}});
  vm.runInContext(readFileSync(join(__dirname,'../../web/static/register-invitation.js'),'utf8'),context);
  const fill=()=>{n.get('joinUsername').value='ordinary_user';n.get('joinEmail').value='unverified@example.invalid';n.get('joinPassword').value='Z9'+String(987654321);n.get('joinConfirm').value=n.get('joinPassword').value;};
  return {node:id=>n.get(id),events,history,fill,submit:()=>n.get('joinForm').listeners.submit({preventDefault(){}})};
}
test('preview parsing removes the fragment and never automatically redeems or persists a capability',()=>{
  let calls=0;const h=recipient(async()=>{calls++;throw Error()});assert.equal(calls,0);assert.deepEqual(h.history[0],[null,'','/join']);
  h.node('joinStart').listeners.click();assert.equal(calls,0);assert.equal(h.node('joinForm').hidden,false);
});
test('explicit registration sends only chosen credentials and preserves unverified-email semantics',async()=>{
  let submitted;const h=recipient(async(path,options)=>{assert.equal(path,'/registration-invitations/redeem');submitted=JSON.parse(options.body);return {status:201,json:async()=>({status:'registered',email_verified:false,registration_method:'administrator_invitation'})}});
  h.fill();await h.submit();assert.deepEqual(Object.keys(submitted).sort(),['email','password','token','username']);
  assert.equal(h.node('joinPassword').value,'');assert.equal(h.node('joinConfirm').value,'');assert.equal(h.node('joinForm').hidden,true);
  assert.match(h.node('joinStatus').textContent,/contact email remains unverified/);
});
test('unknown COMMIT outcome clears credentials and prevents automatic replay',async()=>{
  let calls=0;const h=recipient(async()=>{calls++;return {status:503,json:async()=>({error:'outcome_unconfirmed',message:'PRIVATE server detail'})}});
  h.fill();await h.submit();await h.submit();assert.equal(calls,1);assert.equal(h.node('joinPassword').value,'');
  assert.match(h.node('joinStatus').textContent,/Try signing in/);assert.doesNotMatch(h.node('joinStatus').textContent,/PRIVATE/);
});
test('page departure aborts redemption and rejects its late success response',async()=>{
  let finish,signal;const body=new Promise(resolve=>finish=resolve);const h=recipient(async(_,options)=>{signal=options.signal;return body});
  h.fill();const work=h.submit();h.events.pagehide();assert.equal(signal.aborted,true);
  finish({status:201,json:async()=>({status:'registered',email_verified:false,registration_method:'administrator_invitation'})});await work;
  assert.equal(h.node('joinPassword').value,'');assert.doesNotMatch(h.node('joinStatus').textContent,/account is ready/);
});
