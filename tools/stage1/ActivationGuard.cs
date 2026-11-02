using System;
using System.IO;
using System.Linq;
using System.Text;
using System.Collections.Generic;
using System.Security.Cryptography;
using System.Diagnostics;
using System.IO.Pipes;
using System.Security.AccessControl;
using System.Security.Principal;
using System.Web.Script.Serialization;
namespace Sfd.Stage1 {
 public sealed class Target { public string path; public string sha256; }
 public sealed class Lease : IDisposable {
  readonly List<FileStream> handles = new List<FileStream>();
  readonly DateTime expires;
  public readonly string Digest;
  public int Count { get { return handles.Count; } }
  static string Hex(byte[] b) { return BitConverter.ToString(b).Replace("-", "").ToLowerInvariant(); }
  public Lease(Target[] targets, DateTime until) {
   if(targets==null || targets.Length==0 || targets.Length>32 || until.Kind!=DateTimeKind.Utc || until<=DateTime.UtcNow || until>DateTime.UtcNow.AddHours(8)) throw new InvalidOperationException("lease-scope");
   expires=until;
   try {
    var lines=new List<string>(); var seen=new HashSet<string>(StringComparer.OrdinalIgnoreCase);
    foreach(var t in targets.OrderBy(x=>x.path,StringComparer.OrdinalIgnoreCase)) {
     string full=Path.GetFullPath(t.path);
     for(string parent=full; !String.IsNullOrEmpty(parent); parent=Path.GetDirectoryName(parent)) {
      if((File.GetAttributes(parent)&FileAttributes.ReparsePoint)!=0) throw new InvalidOperationException("target-parent-reparse");
     }
     if(full!=t.path || !seen.Add(full) || full.IndexOf(':',2)>=0 || (File.GetAttributes(full)&FileAttributes.ReparsePoint)!=0) throw new InvalidOperationException("target-path");
     var f=new FileStream(full,FileMode.Open,FileAccess.Read,FileShare.None);
     handles.Add(f); string actual; using(var h=SHA256.Create()) actual=Hex(h.ComputeHash(f));
     if(actual!=t.sha256) throw new InvalidOperationException("target-identity");
     lines.Add(full.ToUpperInvariant()+"="+actual);
    }
    using(var h=SHA256.Create()) Digest=Hex(h.ComputeHash(Encoding.UTF8.GetBytes(String.Join("\n",lines))));
   } catch { Dispose(); throw; }
  }
  public bool Healthy { get { return DateTime.UtcNow<expires && handles.Count>0 && handles.All(f=>!f.SafeFileHandle.IsClosed && !f.SafeFileHandle.IsInvalid); } }
  public void Dispose() { foreach(var f in handles) f.Dispose(); handles.Clear(); }
 }
 public static class Guardian {
  public static bool Allowed(string p) {
   string full=Path.GetFullPath(p); if(full!=p || p.IndexOf(':',2)>=0) return false;
   string docker=Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles),"Docker","Docker")+Path.DirectorySeparatorChar;
   string wsl=Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.ProgramFiles),"WSL")+Path.DirectorySeparatorChar;
   string system=Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.Windows),"System32","wsl.exe");
   if(String.Equals(p,system,StringComparison.OrdinalIgnoreCase)) return true;
   if(p.StartsWith(docker,StringComparison.OrdinalIgnoreCase)) return new[]{"Docker Desktop.exe","com.docker.backend.exe","com.docker.service","com.docker.service.exe"}.Contains(Path.GetFileName(p),StringComparer.OrdinalIgnoreCase);
   if(p.StartsWith(wsl,StringComparison.OrdinalIgnoreCase)) return new[]{"wsl.exe","wslservice.exe","wslhost.exe"}.Contains(Path.GetFileName(p),StringComparer.OrdinalIgnoreCase);
   return false;
  }
  public static int Main(string[] args) {
   // Private stdin carries only approved public binary identities and a lease,
   // never credentials or original data. Host controller must authorise first.
   if(args.Length!=1 || !System.Text.RegularExpressions.Regex.IsMatch(args[0],"^sfd-stage1-[a-f0-9]{32}$")) return 2;
   try {
    string line=Console.ReadLine(); if(line==null || line.Length>65536) return 2;
    var j=new JavaScriptSerializer();var data=j.Deserialize<Dictionary<string,object>>(line);
    Target[] targets=j.ConvertToType<Target[]>(data["targets"]);
    if(!targets.All(t=>Allowed(t.path))) return 2;
    DateTime until=DateTime.Parse((string)data["expires_utc"],null,System.Globalization.DateTimeStyles.RoundtripKind).ToUniversalTime();
    using(var lease=new Lease(targets,until)) {
     Console.WriteLine(j.Serialize(new {state="LOCKED",pid=Process.GetCurrentProcess().Id,identity=lease.Digest,count=lease.Count}));
     var security=new PipeSecurity();var sid=WindowsIdentity.GetCurrent().User;
     security.SetAccessRuleProtection(true,false);security.AddAccessRule(new PipeAccessRule(sid,PipeAccessRights.FullControl,AccessControlType.Allow));
     while(lease.Healthy) {
      using(var pipe=new NamedPipeServerStream(args[0],PipeDirection.InOut,1,PipeTransmissionMode.Byte,PipeOptions.Asynchronous,4096,4096,security)) {
       var pending=pipe.BeginWaitForConnection(null,null);
       using(var wait=pending.AsyncWaitHandle) {
        double remaining=(until-DateTime.UtcNow).TotalSeconds;
        if(remaining<=0 || !wait.WaitOne(TimeSpan.FromSeconds(Math.Min(15,remaining)))) continue;
       }
       pipe.EndWaitForConnection(pending);
       using(var reader=new StreamReader(pipe,Encoding.UTF8,false,4096,true)) using(var writer=new StreamWriter(pipe,new UTF8Encoding(false),4096,true)) {
        var read=reader.ReadLineAsync();if(!read.Wait(2000)) continue;string challenge=read.Result;
        if(challenge==null || !System.Text.RegularExpressions.Regex.IsMatch(challenge,"^[a-f0-9]{64}$")) continue;
        writer.WriteLine(j.Serialize(new {challenge=challenge,healthy=lease.Healthy,identity=lease.Digest,count=lease.Count,pid=Process.GetCurrentProcess().Id}));writer.Flush();
       }
      }
     }
    }
    return 0;
   } catch { Console.Error.WriteLine("STOP: activation-lock-guard");return 2; }
  }
 }
}
