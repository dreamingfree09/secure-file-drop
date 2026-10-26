using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Drawing;
using System.IO;
using System.Security.Cryptography;
using System.Text;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;

public sealed class OwnerStagedInput : Form {
 readonly Label status=new Label();
 readonly CheckBox idle=new CheckBox(),reveal=new CheckBox();
 readonly TextBox admin=new TextBox(),username=new TextBox(),generated=new TextBox();
 readonly Button submit=new Button(),copy=new Button();
 readonly Timer expiry=new Timer{Interval=900000};
 readonly JavaScriptSerializer json=new JavaScriptSerializer{MaxJsonLength=65536};
 readonly Dictionary<string,object> cfg;
 Process child;bool handedOff;
 public OwnerStagedInput(Dictionary<string,object> context){
  cfg=context;Text="Secure File Drop — staged private acceptance";
  ClientSize=new Size(670,365);StartPosition=FormStartPosition.CenterScreen;FormBorderStyle=FormBorderStyle.FixedDialog;MaximizeBox=false;
  status.SetBounds(20,15,630,70);status.Text="Existing administrator only. Stop all Secure File Drop transfers, confirm IDLE and enter the existing administrator password locally.";Controls.Add(status);
  idle.SetBounds(20,90,630,25);idle.Text="I confirm transfers are IDLE and will keep this window open.";Controls.Add(idle);
  Add("Existing administrator password",admin,130);admin.UseSystemPasswordChar=true;
  submit.SetBounds(365,171,285,34);submit.Text="Verify administrator locally";submit.Click+=delegate{Submit();};Controls.Add(submit);
  Add("Temporary acceptance username",username,225);username.ReadOnly=true;
  Add("Temporary acceptance password",generated,267);generated.ReadOnly=true;generated.UseSystemPasswordChar=true;
  reveal.SetBounds(20,315,215,25);reveal.Text="Reveal Password";reveal.Enabled=false;reveal.CheckedChanged+=delegate{generated.UseSystemPasswordChar=!reveal.Checked;};Controls.Add(reveal);
  copy.SetBounds(420,310,230,32);copy.Text="Copy Password";copy.Enabled=false;copy.Click+=delegate{if(handedOff&&generated.Text.Length>0){Clipboard.SetText(generated.Text);status.Text="Password copied by your action. Clipboard contents are sensitive. This is the active temporary acceptance account.";}};Controls.Add(copy);
  expiry.Tick+=delegate{ClearGenerated();status.Text="Local credential display expired. No account was reset or deleted.";};
  FormClosing+=delegate{admin.Clear();ClearGenerated();if(child!=null&&!child.HasExited){try{child.StandardInput.Close();}catch(Exception){}}};
 }
 void Add(string label,TextBox box,int y){var l=new Label{Text=label};l.SetBounds(20,y,300,25);Controls.Add(l);box.SetBounds(335,y,315,25);Controls.Add(box);}
 void ClearGenerated(){expiry.Stop();generated.Clear();reveal.Checked=false;reveal.Enabled=copy.Enabled=false;handedOff=false;}
 static string Hash(string path){using(var s=SHA256.Create())using(var f=File.OpenRead(path))return BitConverter.ToString(s.ComputeHash(f)).Replace("-","").ToLowerInvariant();}
 void Change(Action a){if(!IsDisposed&&IsHandleCreated)BeginInvoke(a);}
 void Submit(){
  if(!idle.Checked||admin.Text.Length==0){status.Text="Confirm IDLE and enter the existing administrator password. No ordinary-user password is needed.";return;}
  try{
   if(cfg==null||(bool)cfg["execution_authorised"]!=true||Hash((string)cfg["Windows_driver"])!=(string)cfg["driver_sha256"])throw new InvalidOperationException();
   string driver=(string)cfg["Wsl_driver"],context=(string)cfg["Wsl_context"];
   if(driver.IndexOfAny(new[]{'"','\r','\n'})>=0||context.IndexOfAny(new[]{'"','\r','\n'})>=0)throw new InvalidOperationException();
   child=new Process{StartInfo=new ProcessStartInfo{
    FileName=Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.Windows),"System32","wsl.exe"),
    Arguments="-d Ubuntu -- python3 -B \""+driver+"\" \""+context+"\"",
    UseShellExecute=false,CreateNoWindow=true,RedirectStandardInput=true,RedirectStandardOutput=true,RedirectStandardError=true}};
   child.Start();
   byte[] input=new UTF8Encoding(false).GetBytes(json.Serialize(new{idle=true,Windows_PID=Process.GetCurrentProcess().Id,admin=admin.Text})+"\n");
   try{child.StandardInput.BaseStream.Write(input,0,input.Length);child.StandardInput.BaseStream.Flush();}finally{Array.Clear(input,0,input.Length);admin.Clear();}
   admin.Enabled=submit.Enabled=idle.Enabled=false;status.Text="Verifying administrator privately. Keep this window open for the governed recovery and backend-only adoption.";
   // Read private child streams without exposing them to a terminal, chat or log.
   Task.Run(delegate{
    string line;while((line=child.StandardOutput.ReadLine())!=null){
     if(line=="ADMIN_VERIFIED_WAITING_FOR_ADOPTION")Change(delegate{status.Text="Administrator verified locally. Codex is waiting for all preservation gates before backend-only adoption.";});
     else if(line.StartsWith("{",StringComparison.Ordinal)){
      var value=json.Deserialize<Dictionary<string,object>>(line);line=null;
      if((string)value["event"]!="ACCOUNT_ACTIVE")throw new InvalidOperationException();
      string name=(string)value["username"],password=(string)value["password"];value.Clear();
      Change(delegate{username.Text=name;generated.Text=password;password=null;handedOff=true;reveal.Enabled=copy.Enabled=true;expiry.Start();status.Text="ACTIVE temporary ordinary acceptance account. Genuine login and file integrity checks passed. Password is masked; Reveal or Copy only if you choose.";});
     }
    }
    child.WaitForExit();Change(delegate{if(child.ExitCode!=0&&!handedOff)status.Text="Stopped at a safety gate. No credential values are disclosed; return to Codex.";});
   });
   Task.Run(delegate{while(child.StandardError.ReadLine()!=null){/* Never persist child diagnostics. */}});
  }catch(Exception){admin.Clear();status.Text="Private local workflow could not start. No password is disclosed. Return to Codex.";}
 }
 public bool SelfTest(){
  if(!admin.UseSystemPasswordChar||!generated.UseSystemPasswordChar||!generated.ReadOnly||!username.ReadOnly||copy.Enabled||reveal.Enabled)return false;
  Submit();if(child!=null)return false;
  generated.Text="synthetic-fixture";reveal.Checked=true;if(generated.UseSystemPasswordChar)return false;ClearGenerated();
  return generated.Text.Length==0&&generated.UseSystemPasswordChar&&!copy.Enabled&&!reveal.Enabled;
 }
 [STAThread]public static int Main(string[] args){
  Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);
  if(args.Length==1&&args[0]=="--self-test"){using(var f=new OwnerStagedInput(null))return f.SelfTest()?0:1;}
  if(args.Length!=1)return 2;
  try{var cfg=new JavaScriptSerializer().Deserialize<Dictionary<string,object>>(File.ReadAllText(args[0]));
   if((bool)cfg["execution_authorised"]!=true)return 3;
   using(var f=new OwnerStagedInput(cfg))Application.Run(f);return 0;
  }catch(Exception){return 1;}
 }
}
