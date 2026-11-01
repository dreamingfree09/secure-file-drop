using System;
using System.IO;
using System.IO.Pipes;
using System.Text;
using System.Security.Cryptography;
using System.Security.Principal;
using System.Security.AccessControl;
using System.Diagnostics;
using System.Runtime.InteropServices;
using System.Runtime.Versioning;
using System.Web.Script.Serialization;
using System.Windows.Forms;
[assembly:TargetFramework(".NETFramework,Version=v4.8",FrameworkDisplayName=".NET Framework 4.8")]
namespace SfdSyntheticInput {
 public static class Program {
  static readonly string[] Scopes={"create_issuer","install_current_user_root","select_endpoint","retire_old_root"};
  static void Need(bool yes){if(!yes)throw new InvalidOperationException("FIXTURE_INPUT_GATE");}
  static string Hash(string s){using(var h=SHA256.Create())return BitConverter.ToString(h.ComputeHash(Encoding.UTF8.GetBytes(s))).Replace("-","").ToLowerInvariant();}
  static string SidHash(){return Hash(WindowsIdentity.GetCurrent().User.Value);}
  static void Validate(System.Collections.Generic.Dictionary<string,object> c){
   Need(c.Count==10 && (string)c["schema"]=="SFD_SYNTHETIC_SECRET_INPUT_V1" && (bool)c["fixture_only"] &&
        !(bool)c["real_actions"] && Array.IndexOf(Scopes,(string)c["scope"])>=0);
   Need((string)c["owner_sid_sha256"]==SidHash());
   DateTime expiry;Need(DateTime.TryParse((string)c["expires"],null,System.Globalization.DateTimeStyles.RoundtripKind,out expiry));
   Need(expiry.ToUniversalTime()>DateTime.UtcNow && expiry.ToUniversalTime()<=DateTime.UtcNow.AddMinutes(2));
   Need(Convert.ToInt32(c["parent_pid"])==ParentId() && Process.GetProcessById(ParentId()).SessionId==Process.GetCurrentProcess().SessionId);
   FixtureRoot((string)c["fixture_root"]);
   Guid lease;Need(Guid.TryParse((string)c["lease_id"],out lease));
   string review=(string)c["public_review_text"];
   Need(review.Length<=8192 && !review.Contains("\0") && review.Contains("SYNTHETIC") &&
        review.Contains("No real action") && review.Contains((string)c["scope"]));
  }

  static string FixtureRoot(string path){
   string full=Path.GetFullPath(path).TrimEnd(Path.DirectorySeparatorChar);Guid nonce=Guid.Empty;
   Need(String.Equals(Path.GetDirectoryName(full),Path.GetTempPath().TrimEnd(Path.DirectorySeparatorChar),StringComparison.OrdinalIgnoreCase));
   string name=Path.GetFileName(full);
   Need(name.StartsWith("sfd-pki-fixtures-",StringComparison.Ordinal)&&Guid.TryParse(name.Substring(17),out nonce));
   Need((File.GetAttributes(full)&FileAttributes.ReparsePoint)==0);
   var marker=new JavaScriptSerializer().DeserializeObject(File.ReadAllText(Path.Combine(full,"fixture.json"))) as System.Collections.Generic.Dictionary<string,object>;
   Need(marker!=null&&marker.Count==3&&(string)marker["schema"]=="SFD_DISPOSABLE_PKI_FIXTURE_V1"&&(bool)marker["synthetic_only"]&&(string)marker["nonce"]==nonce.ToString());
   return full;
  }
  static object ProtectRoot(string path){
   path=FixtureRoot(path);var owner=WindowsIdentity.GetCurrent().User;
   var acl=new DirectorySecurity();acl.SetOwner(owner);acl.SetAccessRuleProtection(true,false);
   foreach(var sid in new[]{owner,new SecurityIdentifier(WellKnownSidType.LocalSystemSid,null)})
    acl.AddAccessRule(new FileSystemAccessRule(sid,FileSystemRights.FullControl,InheritanceFlags.ContainerInherit|InheritanceFlags.ObjectInherit,PropagationFlags.None,AccessControlType.Allow));
   Directory.SetAccessControl(path,acl);
   Need(Directory.GetAccessControl(path).AreAccessRulesProtected);
   return new{new_fixture_ACL_protected=true};
  }
  static void Consume(System.Collections.Generic.Dictionary<string,object> c){
   string root=FixtureRoot((string)c["fixture_root"]);
   var acl=Directory.GetAccessControl(root);Need(acl.AreAccessRulesProtected&&acl.GetOwner(typeof(SecurityIdentifier)).Equals(WindowsIdentity.GetCurrent().User));
   string name="input-"+Guid.Parse((string)c["lease_id"]).ToString()+".used";
   using(var stream=new FileStream(Path.Combine(root,name),FileMode.CreateNew,FileAccess.Write,FileShare.None)){
    byte[] marker=Encoding.UTF8.GetBytes("SYNTHETIC ONE USE; "+(string)c["scope"]);stream.Write(marker,0,marker.Length);stream.Flush(true);
   }
  }

  [StructLayout(LayoutKind.Sequential,CharSet=CharSet.Unicode)]
  struct Entry { public uint size,usage,pid;public IntPtr heap;public uint module,threads,parent;public int priority;public uint flags;[MarshalAs(UnmanagedType.ByValTStr,SizeConst=260)]public string exe; }
  [DllImport("kernel32.dll",SetLastError=true)]static extern IntPtr CreateToolhelp32Snapshot(uint flags,uint pid);
  [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)]static extern bool Process32FirstW(IntPtr snap,ref Entry e);
  [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)]static extern bool Process32NextW(IntPtr snap,ref Entry e);
  [DllImport("kernel32.dll")]static extern bool CloseHandle(IntPtr h);
  static int ParentId(){
   IntPtr snap=CreateToolhelp32Snapshot(2,0);Need(snap!=new IntPtr(-1));
   try{var e=new Entry{size=(uint)Marshal.SizeOf(typeof(Entry))};
    if(Process32FirstW(snap,ref e))do{if(e.pid==(uint)Process.GetCurrentProcess().Id)return (int)e.parent;}while(Process32NextW(snap,ref e));
   }finally{CloseHandle(snap);}throw new InvalidOperationException("PARENT_UNAVAILABLE");
  }
  public sealed class InputForm:Form {
   internal readonly TextBox First=new TextBox{UseSystemPasswordChar=true,ShortcutsEnabled=false,MaxLength=128,Dock=DockStyle.Top};
   internal readonly TextBox Confirm=new TextBox{UseSystemPasswordChar=true,ShortcutsEnabled=false,MaxLength=128,Dock=DockStyle.Top};
   internal readonly CheckBox Consent=new CheckBox{Text="I confirm only this displayed SYNTHETIC phase; no real action is authorised.",Dock=DockStyle.Top,Height=45};
   internal readonly Button Send=new Button{Text="Send fixture secret through private local pipe",Dock=DockStyle.Bottom,Enabled=false};
   internal bool Accepted;
   internal readonly bool RequiresSecret;
   readonly Timer clock=new Timer{Interval=200};
   public InputForm(string scope,string review,DateTime expires){
    RequiresSecret=scope=="create_issuer";First.Enabled=RequiresSecret;Confirm.Enabled=RequiresSecret;First.Visible=RequiresSecret;Confirm.Visible=RequiresSecret;
    Send.Text=RequiresSecret?"Send fixture secret through private local pipe":"Confirm only this fixture consent — no secret";
    Text="SFD SYNTHETIC INPUT — DO NOT ENTER REAL SECRETS";Width=820;Height=580;FormBorderStyle=FormBorderStyle.FixedDialog;MaximizeBox=false;
    var info=new TextBox{ReadOnly=true,Multiline=true,ScrollBars=ScrollBars.Vertical,Dock=DockStyle.Fill,
      Text="PREPARATION ONLY. No CA, trust, publisher, endpoint or retirement action can run.\r\nPhase: "+scope+"\r\n"+review};
    var cancel=new Button{Text="Cancel",Dock=DockStyle.Bottom,DialogResult=DialogResult.Cancel};
    Controls.Add(info);Controls.Add(Confirm);Controls.Add(First);Controls.Add(Consent);Controls.Add(Send);Controls.Add(cancel);
    CancelButton=cancel;
    EventHandler changed=(s,e)=>Send.Enabled=Consent.Checked && DateTime.UtcNow<expires && (!RequiresSecret || (First.Text.Length>=32 &&
       First.Text.Length<=128 && First.Text==Confirm.Text));
    First.TextChanged+=changed;Confirm.TextChanged+=changed;Consent.CheckedChanged+=changed;
    Send.Click+=(s,e)=>{if(Send.Enabled){Accepted=true;DialogResult=DialogResult.OK;Close();}};
    clock.Tick+=(s,e)=>{if(DateTime.UtcNow>=expires){Accepted=false;First.Clear();Confirm.Clear();Close();}};
    clock.Start();
   }
   protected override void Dispose(bool disposing){if(disposing){clock.Stop();clock.Dispose();First.Clear();Confirm.Clear();}base.Dispose(disposing);}
  }
  static object ControlsTest(){
   foreach(string scope in Scopes)using(var f=new InputForm(scope,"SYNTHETIC "+scope+" No real action",DateTime.UtcNow.AddMinutes(1))){
    Need(!f.Send.Enabled && !f.Consent.Checked && f.First.UseSystemPasswordChar && f.Confirm.UseSystemPasswordChar && !f.First.ShortcutsEnabled);
    if(f.RequiresSecret){
     f.First.Text=new string('x',32);f.Confirm.Text=new string('y',32);f.Consent.Checked=true;Need(!f.Send.Enabled);
     f.Confirm.Text=f.First.Text;Need(f.Send.Enabled);
    }else{
     Need(!f.First.Enabled&&!f.Confirm.Enabled);f.Consent.Checked=true;Need(f.Send.Enabled);
    }
    f.Consent.Checked=false;Need(!f.Send.Enabled);
   }
   return new{four_scopes=true,masked=true,confirm_required=true,consent_not_preselected=true,clipboard_shortcuts_disabled=true,real_actions_enabled=false,no_secret_on_other_scopes=true};
  }
  static int Transfer(string mode,string inputHandle,string outputHandle){
   using(var source=new AnonymousPipeClientStream(PipeDirection.In,inputHandle))
   using(var dest=new AnonymousPipeClientStream(PipeDirection.Out,outputHandle)){
    var reader=new BinaryReader(source,Encoding.UTF8);int count=reader.ReadInt32();Need(count>0&&count<=16384);
    byte[] raw=reader.ReadBytes(count);Need(raw.Length==count);
    var c=new JavaScriptSerializer().DeserializeObject(Encoding.UTF8.GetString(raw)) as System.Collections.Generic.Dictionary<string,object>;
    Array.Clear(raw,0,raw.Length);Need(c!=null);Validate(c);Consume(c);
    if(mode=="--cancel-selftest")return 2;
    byte[] secret=null;
    try{
     bool needsSecret=(string)c["scope"]=="create_issuer";
     if(mode=="--pipe-selftest")secret=needsSecret?Encoding.UTF8.GetBytes("SYNTHETIC-ONLY-"+Guid.NewGuid().ToString("N")):new byte[0];
     else{
      Need(mode=="--interactive-fixture" && Environment.UserInteractive);
      DateTime expires=DateTime.Parse((string)c["expires"]).ToUniversalTime();
      using(var form=new InputForm((string)c["scope"],(string)c["public_review_text"],expires)){
       if(form.ShowDialog()!=DialogResult.OK || !form.Accepted)return 2;
       secret=needsSecret?Encoding.UTF8.GetBytes(form.First.Text):new byte[0];
      }
     }
     Validate(c);
     var writer=new BinaryWriter(dest,Encoding.UTF8);writer.Write(secret.Length);writer.Write(secret);writer.Flush();
     Console.WriteLine(new JavaScriptSerializer().Serialize(new{private_pipe_transfer=true,real_action=false,secret_required=needsSecret}));
     return 0;
    }finally{if(secret!=null)Array.Clear(secret,0,secret.Length);}
   }
  }
  [STAThread]public static int Main(string[] args){
   try{
    if(args.Length==1&&args[0]=="--identity"){Console.WriteLine(new JavaScriptSerializer().Serialize(new{sid_sha256=SidHash(),session=Process.GetCurrentProcess().SessionId}));return 0;}
    if(args.Length==2&&args[0]=="--protect-fixture-root"){Console.WriteLine(new JavaScriptSerializer().Serialize(ProtectRoot(args[1])));return 0;}
    if(args.Length==1&&args[0]=="--self-test"){Console.WriteLine(new JavaScriptSerializer().Serialize(ControlsTest()));return 0;}
    if(args.Length==3&&(args[0]=="--pipe-selftest"||args[0]=="--interactive-fixture"||args[0]=="--cancel-selftest"))return Transfer(args[0],args[1],args[2]);
    throw new InvalidOperationException("NO_REAL_SECRET_ENTRY_ROUTE");
   }catch(Exception){Console.Error.WriteLine("STOP: synthetic input gate failed; no values disclosed.");return 1;}
  }
 }
}
