const assert=require('node:assert/strict');
const fs=require('node:fs');
const path=require('node:path');
const vm=require('node:vm');
const {test}=require('node:test');
function harness(){
 const nodes=new Map(),requests=[];
 function node(id){if(!nodes.has(id)){const cls=new Set();nodes.set(id,{value:'',textContent:'',innerHTML:'',style:{},files:[],addEventListener(){},classList:{add(x){cls.add(x)},remove(x){cls.delete(x)},toggle(x,v){if(v)cls.add(x);else cls.delete(x)},contains(x){return cls.has(x)}}})}return nodes.get(id)}
 class XHR{constructor(){this.listeners={};this.upload={addEventListener:(type,fn)=>{this.progress=fn}};requests.push(this)}addEventListener(type,fn){this.listeners[type]=fn}open(){}send(){}abort(){this.aborted=true;this.listeners.abort?.()}}
 const context=vm.createContext({document:{readyState:'complete',getElementById:node,querySelectorAll:()=>[]},fetch:async url=>url==='/files'?{ok:true,json:async()=>({id:'synthetic'})}:{ok:true,status:204},console,AbortController,clearTimeout,TextEncoder,XMLHttpRequest:XHR,FormData:class{append(){}},setTimeout(){}});
 const html=fs.readFileSync(path.join(__dirname,'../../web/static/index.html'),'utf8');
 vm.runInContext(html.split('<script>')[1].split('</script>')[0],context);
 return{context,node,requests};
}
test('assessment observation: logout clears registration passwords',async()=>{
 const h=harness();
 h.node('regPassword').value='SyntheticPass123';h.node('regPasswordConfirm').value='SyntheticPass123';
 vm.runInContext('isLoggedIn=true;sessionGeneration=1;',h.context);
 await vm.runInContext('logout()',h.context);
 assert.equal(h.node('regPassword').value,'');
 assert.equal(h.node('regPasswordConfirm').value,'');
});
test('assessment observation: old XHR is aborted and cannot mutate later-session UI',async()=>{
 const h=harness();
 vm.runInContext('isLoggedIn=true;sessionGeneration=1;selectedFile={name:"fixture.bin",size:8,type:"application/octet-stream"};',h.context);
 const pending=vm.runInContext('uploadFile()',h.context);
 await new Promise(resolve=>setImmediate(resolve));
 assert.equal(h.requests.length,1);
 await vm.runInContext('logout()',h.context);
 vm.runInContext('isLoggedIn=true;sessionGeneration++;',h.context);
 h.node('progressText').textContent='New session';
 h.requests[0].progress({lengthComputable:true,loaded:7*1024*1024,total:8*1024*1024});
 assert.equal(h.node('progressText').textContent,'New session');
 assert.equal(h.requests[0].aborted,true);
 h.requests[0].status=200;h.requests[0].listeners.load();await pending;
});
