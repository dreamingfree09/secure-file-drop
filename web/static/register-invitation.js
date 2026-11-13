(() => {
  'use strict';
  const $=id=>document.getElementById(id);
  const parameters=new URLSearchParams(location.hash.slice(1));
  let token=parameters.get('token')||'', busy=false, controller;
  const valid=parameters.getAll('token').length===1 && [...parameters.keys()].every(k=>k==='token') && /^[a-f0-9]{64}$/.test(token);
  history.replaceState(null,'',location.pathname);
  function message(text,error=false){$('joinStatus').textContent=text;$('joinStatus').dataset.error=String(error);}
  function clearPasswords(){$('joinPassword').value='';$('joinConfirm').value='';}
  function clear(){token='';controller?.abort();$('joinForm').reset();clearPasswords();$('joinForm').hidden=true;$('joinStart').hidden=true;}
  if(!valid){clear();message('This invitation is unavailable. Ask the administrator for a new link.',true);}
  $('joinStart').addEventListener('click',()=>{if(!token)return;$('joinStart').hidden=true;$('joinForm').hidden=false;$('joinUsername').focus();});
  $('joinForm').addEventListener('submit',async event=>{
    event.preventDefault();if(busy||!token)return;
    if($('joinPassword').value!==$('joinConfirm').value){message('The passwords do not match.',true);clearPasswords();return;}
    busy=true;controller=new AbortController();$('joinSubmit').disabled=true;
    const timeout=setTimeout(()=>controller.abort(),15000);
    const input={token,username:$('joinUsername').value,email:$('joinEmail').value,password:$('joinPassword').value};
    clearPasswords();message('Creating your account…');
    try{
      const response=await fetch('/registration-invitations/redeem',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(input),signal:controller.signal});
      const body=await response.json().catch(()=>({}));
      if(!token)return;
      if(response.status===201 && body.status==='registered' && body.email_verified===false && body.registration_method==='administrator_invitation'){
        clear();message('Your account is ready. Sign in with your chosen username and password. Your contact email remains unverified.');$('joinSignIn').hidden=false;
      }else if(response.status===410){clear();message('This invitation is unavailable, expired or already used. Ask the administrator for a new link.',true);}
      else if(body.error==='outcome_unconfirmed'){clear();message('Account creation could not be confirmed. Try signing in with your chosen credentials before reopening the original invitation.',true);$('joinSignIn').hidden=false;}
      else if(response.status===400){message('Check your username, contact email and password requirements.',true);}
      else if(response.status===409){message('Registration could not be completed with those details. Try different details or contact the administrator.',true);}
      else if(response.status===429){message('Too many attempts. Wait a minute before trying again.',true);}
      else{message('Registration is temporarily unavailable. Try signing in first if the previous attempt may have completed.',true);}
    }catch{if(token)message('The result could not be confirmed. Try signing in before retrying; never submit repeatedly.',true);$('joinSignIn').hidden=false;}
    finally{clearTimeout(timeout);input.password='';input.token='';busy=false;$('joinSubmit').disabled=false;}
  });
  addEventListener('pagehide',clear);
  addEventListener('pageshow',event=>{if(event.persisted){clear();message('Reopen the original invitation to continue.',true);}});
})();
