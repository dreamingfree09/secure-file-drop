using System;
using System.Collections.Generic;
using System.Net;
using System.Net.Http;
using System.Reflection;
using System.Text;
using System.Threading;
using System.Web.Script.Serialization;
using SecureFileDrop.Desktop;

// Private parent/child stdin/stdout only. Never run this console tool in a terminal
// with genuine credentials. The native form owns its redirected streams.
static class StrictTransport {
 static readonly JavaScriptSerializer Json=new JavaScriptSerializer{MaxJsonLength=32768};
 static HttpClient Raw(SfdClient c){return (HttpClient)typeof(SfdClient).GetField("http",BindingFlags.Instance|BindingFlags.NonPublic).GetValue(c);}
 static object Execute(SfdClient admin,Dictionary<string,object> x){
  string op=(string)x["op"];
  if(op=="policy"){using(var h=(HttpClientHandler)typeof(SfdClient).GetMethod("CreateHandler",BindingFlags.NonPublic|BindingFlags.Static).Invoke(null,new object[]{new CookieContainer()}))return new{ok=ServicePointManager.ServerCertificateValidationCallback==null&&ServicePointManager.SecurityProtocol==SecurityProtocolType.SystemDefault&&!h.AllowAutoRedirect&&h.UseCookies,revocation=h.CheckCertificateRevocationList};}
  if(op=="admin"){
   admin.Login((string)x["username"],(string)x["password"],CancellationToken.None).GetAwaiter().GetResult();
   if(!admin.Authenticated||!admin.Probe(CancellationToken.None).GetAwaiter().GetResult())throw new InvalidOperationException();
   using(var budget=new CancellationTokenSource(TimeSpan.FromSeconds(12)))
   using(var meResponse=Raw(admin).GetAsync(new Uri(admin.Origin,"me"),budget.Token).GetAwaiter().GetResult()){
    if(meResponse.StatusCode!=HttpStatusCode.OK)throw new InvalidOperationException();
    var me=Json.Deserialize<Dictionary<string,object>>(meResponse.Content.ReadAsStringAsync().GetAwaiter().GetResult());
    if((string)me["status"]!="ok"||(string)me["username"]!=(string)x["username"]||!(bool)me["is_admin"])throw new InvalidOperationException();
   }
   var jar=(CookieContainer)typeof(SfdClient).GetField("cookies",BindingFlags.Instance|BindingFlags.NonPublic).GetValue(admin);
   return new{ok=true,session=jar.GetCookies(admin.Origin)["sfd_session"].Value};
  }
  if(op=="logout"){admin.Logout(CancellationToken.None).GetAwaiter().GetResult();return new{ok=true};}
  if(op!="register"&&op!="verify")throw new InvalidOperationException();
  string path=op=="register"?"register":(string)x["path"];
  Uri target=new Uri(admin.Origin,path);
  if(target.Scheme!="https"||target.Host!="localhost"||target.Port!=8443||target.UserInfo.Length!=0||target.Fragment.Length!=0||
     (op=="verify"&&target.AbsolutePath!="/verify"))throw new InvalidOperationException();
  using(var budget=new CancellationTokenSource(TimeSpan.FromSeconds(40)))
  using(var req=new HttpRequestMessage(op=="register"?HttpMethod.Post:HttpMethod.Get,target)){
   if(op=="register")req.Content=new StringContent(Json.Serialize(x["body"]),Encoding.UTF8,"application/json");
   using(var resp=Raw(admin).SendAsync(req,budget.Token).GetAwaiter().GetResult()){
    var bytes=resp.Content.ReadAsByteArrayAsync().GetAwaiter().GetResult();
    if(bytes.Length>32768)throw new InvalidOperationException();
    // Verification HTML is not needed and must not be logged.
    object body=op=="register"&&resp.StatusCode==HttpStatusCode.Created?Json.DeserializeObject(Encoding.UTF8.GetString(bytes)):null;
    return new{ok=true,status=(int)resp.StatusCode,body=body};
   }
  }
 }
 static int Main(){
  Console.InputEncoding=new UTF8Encoding(false);Console.OutputEncoding=new UTF8Encoding(false);
  if(ServicePointManager.ServerCertificateValidationCallback!=null||ServicePointManager.SecurityProtocol!=SecurityProtocolType.SystemDefault)return 2;
  using(var admin=new SfdClient(new Settings{Backend="https://localhost:8443",RememberSession=false,MaxUploadBytes=1048576})){
   Raw(admin).MaxResponseContentBufferSize=65536;
   string line;
   while((line=Console.ReadLine())!=null){
    try{if(line.Length>32768)throw new InvalidOperationException();var x=Json.Deserialize<Dictionary<string,object>>(line);line=null;
     Console.WriteLine(Json.Serialize(Execute(admin,x)));x.Clear();
    }catch(Exception){Console.WriteLine("{\"ok\":false}");return 1;}
   }
  }
  return 0;
 }
}
