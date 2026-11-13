(() => {
  'use strict';
  const $=id=>document.getElementById(id), uuid=/^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
  let enabled=false,administrator=false,epoch=0,request=0,busy=false,next='',pending=null,timer;
  const current=(e,g)=>enabled&&administrator&&isLoggedIn&&e===epoch&&g===sessionGeneration;
  function message(text,error=false){$('registrationAdminStatus').textContent=text;$('registrationAdminStatus').dataset.error=String(error);}
  function controls(value){busy=value;for(const id of ['registrationIssue','registrationRefresh','registrationMore','registrationTTL'])$(id).disabled=value;}
  function receipt(){ $('registrationURL').value='';$('registrationReceipt').hidden=true; }
  function reset(){epoch++;request++;administrator=false;pending=null;next='';clearTimeout(timer);receipt();controls(false);
    $('registrationAdminList').replaceChildren();$('registrationAdminPanel').hidden=true;$('registrationMore').hidden=true;$('registrationRevokeYes').disabled=false;
    if($('registrationRevokeDialog').open)$('registrationRevokeDialog').close();message('');}
  function configure(config){enabled=config.registration_invitations_enabled===true;if(!enabled)reset();else if(administrator&&isLoggedIn)void refresh();}
  function activate(identity){reset();administrator=identity.environment_admin===true;if(enabled&&administrator&&isLoggedIn)void refresh();}
  function card(row){
    const node=document.createElement('article');node.className='registration-card';node.dataset.invitationId=row.id;
    const state=document.createElement('span');state.className='registration-badge registration-badge-'+row.state;state.textContent=row.state[0].toUpperCase()+row.state.slice(1);node.append(state);
    for(const text of ['Invitation '+row.id,'Created '+new Date(row.created_at).toLocaleString(),'Expires '+new Date(row.expires_at).toLocaleString()]){const p=document.createElement('p');p.textContent=text;node.append(p);}
    if(row.state==='active'){
      const button=document.createElement('button');button.className='btn btn-danger';button.type='button';button.textContent='Revoke invitation';
      button.addEventListener('click',()=>{if(busy)return;pending={id:row.id,epoch,generation:sessionGeneration};$('registrationRevokeDialog').showModal();$('registrationRevokeCancel').focus();});node.append(button);
    }
    $('registrationAdminList').append(node);
  }
  async function refresh(more=false){
    if(!enabled||!administrator||!isLoggedIn)return;
    const e=epoch,g=sessionGeneration,sequence=++request;clearTimeout(timer);controls(true);$('registrationAdminPanel').hidden=false;
    if(!more){$('registrationAdminList').replaceChildren();next='';$('registrationMore').hidden=true;}
    message('Checking invitation status…');
    try{
      const response=await sessionFetch('/admin/registration-invitations'+(more&&next?'?before='+next:''));
      if(!current(e,g)||sequence!==request)return;
      if(!response.ok)throw Error();const rows=await response.json();if(!current(e,g)||sequence!==request)return;
      if(!Array.isArray(rows)||rows.length>50||!rows.every(row=>uuid.test(row.id)&&['active','expired','revoked','redeemed'].includes(row.state)&&Number.isFinite(Date.parse(row.created_at))&&Number.isFinite(Date.parse(row.expires_at))))throw Error();
      const cursor=response.headers.get('X-Next-Cursor')||'';if(cursor&&!uuid.test(cursor))throw Error();
      rows.forEach(card);next=cursor;$('registrationMore').hidden=!next;
      message($('registrationAdminList').children.length?'Status verified by the server. Times use your local timezone.':'No registration invitations yet.');
    }catch{if(current(e,g)&&sequence===request){$('registrationAdminList').replaceChildren();next='';$('registrationMore').hidden=true;message('Could not verify invitations. Refresh to try again.',true);}}
    finally{if(current(e,g)&&sequence===request){controls(false);timer=setTimeout(()=>void refresh(),60000);}}
  }
  async function issue(){
    if(busy||!enabled||!administrator||!isLoggedIn)return;
    const e=epoch,g=sessionGeneration;clearTimeout(timer);receipt();controls(true);
    try{
      const response=await sessionFetch('/admin/registration-invitations',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({ttl_seconds:Number($('registrationTTL').value)})});
      if(!current(e,g))return;if(response.status!==201)throw Error();const body=await response.json();if(!current(e,g))return;
      const url=new URL(body.url),fragment=new URLSearchParams(url.hash.slice(1));
      if(!uuid.test(body.id)||url.origin!==location.origin||url.pathname!=='/join'||url.search||url.username||url.password||fragment.getAll('token').length!==1||[...fragment.keys()].some(k=>k!=='token')||!/^[a-f0-9]{64}$/.test(fragment.get('token')||''))throw Error();
      await refresh();if(!current(e,g))return;$('registrationURL').value=url.href;$('registrationReceipt').hidden=false;message('Invitation created. Copy it now and deliver it privately. The link cannot be recovered later.');
    }catch{if(current(e,g))message('Invitation creation could not be confirmed. Refresh before issuing another.',true);}
    finally{if(current(e,g))controls(false);}
  }
  async function revoke(){
    const target=pending;if(!target||busy||!current(target.epoch,target.generation))return;
    clearTimeout(timer);controls(true);$('registrationRevokeYes').disabled=true;
    try{
      const response=await sessionFetch('/admin/registration-invitations/'+target.id,{method:'DELETE'});
      if(!current(target.epoch,target.generation))return;$('registrationRevokeDialog').close();pending=null;receipt();
      if(response.status!==204)throw Error();await refresh();
    }catch{if(current(target.epoch,target.generation)){if($('registrationRevokeDialog').open)$('registrationRevokeDialog').close();pending=null;$('registrationAdminList').replaceChildren();message('Revocation could not be confirmed. Refresh to check before delivering the invitation.',true);}}
    finally{if(current(target.epoch,target.generation)){controls(false);$('registrationRevokeYes').disabled=false;}}
  }
  $('registrationIssue').addEventListener('click',()=>void issue());$('registrationRefresh').addEventListener('click',()=>void refresh());$('registrationMore').addEventListener('click',()=>void refresh(true));
  $('registrationRevokeYes').addEventListener('click',()=>void revoke());$('registrationRevokeCancel').addEventListener('click',()=>{pending=null;$('registrationRevokeDialog').close();});$('registrationRevokeDialog').addEventListener('cancel',()=>{pending=null;});
  $('registrationCopy').addEventListener('click',async()=>{const e=epoch,g=sessionGeneration,value=$('registrationURL').value;if(!current(e,g)||!value)return;
    try{await navigator.clipboard.writeText(value);if(current(e,g))message('Invitation copied. Deliver it privately; anyone holding it may register once.');}
    catch{if(current(e,g)){$('registrationURL').select();message('Clipboard unavailable. Copy the selected invitation manually.',true);}}});
  globalThis.SFDRegistrationAdmin={configure,activate,reset,refresh};
  addEventListener('pagehide',reset);
  addEventListener('pageshow',event=>{
    if(!event.persisted)return;
    // A cached document cannot establish that its old cookie/role is current.
    // Drop every account-bound view before fresh server/configuration readback.
    resetSessionUI();void loadConfig();void checkSession();
  });
})();
