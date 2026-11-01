using System;
using System.IO;
using System.Linq;
using System.Runtime.InteropServices;
using System.Runtime.Versioning;
using System.Security.Cryptography.X509Certificates;
using System.Web.Script.Serialization;
[assembly:TargetFramework(".NETFramework,Version=v4.8",FrameworkDisplayName=".NET Framework 4.8")]
namespace SfdMemoryChainFixture {
 public static class Program { static string Phase="start"; static int ApiError=0;
  [StructLayout(LayoutKind.Sequential)]struct EngineConfig{public uint Size;public IntPtr RestrictedRoot,RestrictedTrust,RestrictedOther;public uint AdditionalCount;public IntPtr Additional;public uint Flags,Timeout,MaxCache,Cycle;public IntPtr ExclusiveRoot,ExclusivePeople;public uint ExclusiveFlags;}
  [StructLayout(LayoutKind.Sequential)]struct EnhancedUsage{public uint Count;public IntPtr Oids;}
  [StructLayout(LayoutKind.Sequential)]struct UsageMatch{public uint Type;public EnhancedUsage Usage;}
  [StructLayout(LayoutKind.Sequential)]struct ChainParameters{public uint Size;public UsageMatch RequestedUsage;}
  [StructLayout(LayoutKind.Sequential)]struct TrustStatus{public uint Error,Info;}
  [StructLayout(LayoutKind.Sequential)]struct ChainPrefix{public uint Size;public TrustStatus Trust;public uint Chains;public IntPtr ChainArray;}
  [StructLayout(LayoutKind.Sequential)]struct PolicyParameters{public uint Size,Flags;public IntPtr Extra;}
  [StructLayout(LayoutKind.Sequential)]struct PolicyStatus{public uint Size,Error;public int ChainIndex,ElementIndex;public IntPtr Extra;}
  [StructLayout(LayoutKind.Sequential,CharSet=CharSet.Unicode)]struct SslPolicy{public uint Size,AuthType,Checks;[MarshalAs(UnmanagedType.LPWStr)]public string Name;}
  [DllImport("crypt32.dll",SetLastError=true)]static extern IntPtr CertOpenStore(IntPtr provider,uint encoding,IntPtr crypt,uint flags,IntPtr param);
  [DllImport("crypt32.dll",SetLastError=true)]static extern bool CertCloseStore(IntPtr store,uint flags);
  [DllImport("crypt32.dll",SetLastError=true)]static extern bool CertAddEncodedCertificateToStore(IntPtr store,uint encoding,byte[] data,uint count,uint disposition,IntPtr context);
  [DllImport("crypt32.dll",SetLastError=true)]static extern bool CertAddEncodedCRLToStore(IntPtr store,uint encoding,byte[] data,uint count,uint disposition,IntPtr context);
  [DllImport("crypt32.dll",SetLastError=true)]static extern IntPtr CertCreateCertificateContext(uint encoding,byte[] data,uint count);
  [DllImport("crypt32.dll")]static extern void CertFreeCertificateContext(IntPtr context);
  [DllImport("crypt32.dll",SetLastError=true)]static extern bool CertCreateCertificateChainEngine(ref EngineConfig config,out IntPtr engine);
  [DllImport("crypt32.dll")]static extern void CertFreeCertificateChainEngine(IntPtr engine);
  [DllImport("crypt32.dll",SetLastError=true)]static extern bool CertGetCertificateChain(IntPtr engine,IntPtr leaf,IntPtr time,IntPtr additional,ref ChainParameters parameters,uint flags,IntPtr reserved,out IntPtr chain);
  [DllImport("crypt32.dll")]static extern void CertFreeCertificateChain(IntPtr chain);
  [DllImport("crypt32.dll",SetLastError=true)]static extern bool CertVerifyCertificateChainPolicy(IntPtr policy,IntPtr chain,ref PolicyParameters parameters,ref PolicyStatus status);
  static void Need(bool yes){if(!yes)throw new InvalidOperationException("MEMORY_CHAIN_GATE");}
  static string Fixture(string folder){
   string full=Path.GetFullPath(folder).TrimEnd(Path.DirectorySeparatorChar);Guid id=Guid.Empty;
   Need(String.Equals(Path.GetDirectoryName(full),Path.GetTempPath().TrimEnd(Path.DirectorySeparatorChar),StringComparison.OrdinalIgnoreCase));
   string name=Path.GetFileName(full);Need(name.StartsWith("sfd-pki-fixtures-",StringComparison.Ordinal)&&Guid.TryParse(name.Substring(17),out id));
   Need((File.GetAttributes(full)&FileAttributes.ReparsePoint)==0);
   var m=new JavaScriptSerializer().DeserializeObject(File.ReadAllText(Path.Combine(full,"fixture.json"))) as System.Collections.Generic.Dictionary<string,object>;
   Need(m!=null&&m.Count==3&&(string)m["schema"]=="SFD_DISPOSABLE_PKI_FIXTURE_V1"&&(bool)m["synthetic_only"]&&(string)m["nonce"]==id.ToString());
   return full;
  }
  static byte[] Data(string root,string name){
   Need(new[]{"root.der","foreign-root.der","valid.der","revoked.der","expired.der","client.der","current.crl.der","expired.crl.der","wrong_aki.crl.der","corrupt.crl.der","foreign.crl.der"}.Contains(name));
   string p=Path.Combine(root,name);Need((File.GetAttributes(p)&FileAttributes.ReparsePoint)==0&&new FileInfo(p).Length<=1048576);
   return File.ReadAllBytes(p);
  }
  static IntPtr MemoryStore(){IntPtr s=CertOpenStore(new IntPtr(2),0,IntPtr.Zero,0x2000,IntPtr.Zero);Need(s!=IntPtr.Zero);return s;}
  static object Check(string folder,string rootName,string leafName,string crlName,string host){
   folder=Fixture(folder);Need(host=="localhost"||host=="wrong.invalid"||host=="127.0.0.1");
   byte[] root=Data(folder,rootName),leaf=Data(folder,leafName);
   using(var publicRoot=new X509Certificate2(root)){Need(publicRoot.Subject.StartsWith("CN=SyntheticSFDFixture-",StringComparison.Ordinal)&&!publicRoot.HasPrivateKey);}
   IntPtr roots=IntPtr.Zero,other=IntPtr.Zero,empty=IntPtr.Zero,engine=IntPtr.Zero,cert=IntPtr.Zero,chain=IntPtr.Zero,oid=IntPtr.Zero,oidArray=IntPtr.Zero,extra=IntPtr.Zero;
   try{
    Phase="memory-stores";roots=MemoryStore();other=MemoryStore();empty=MemoryStore();
    Need(CertAddEncodedCertificateToStore(roots,1,root,(uint)root.Length,1,IntPtr.Zero));
    Need(CertAddEncodedCertificateToStore(other,1,root,(uint)root.Length,1,IntPtr.Zero));
    if(crlName!="none"){byte[] c=Data(folder,crlName);Need(CertAddEncodedCRLToStore(other,1,c,(uint)c.Length,1,IntPtr.Zero));}
    // Exclusive in-memory anchors. Application calls only the MEMORY provider; OS chain internals are not instrumented.
    var config=new EngineConfig{Size=(uint)Marshal.SizeOf(typeof(EngineConfig)),RestrictedRoot=IntPtr.Zero,RestrictedTrust=IntPtr.Zero,RestrictedOther=IntPtr.Zero,
      Flags=0,Timeout=0,MaxCache=0,Cycle=0,ExclusiveRoot=roots,ExclusivePeople=empty,ExclusiveFlags=0};
    Phase="create-engine";bool engineOk=CertCreateCertificateChainEngine(ref config,out engine);ApiError=Marshal.GetLastWin32Error();Need(engineOk);
    cert=CertCreateCertificateContext(1,leaf,(uint)leaf.Length);Need(cert!=IntPtr.Zero);
    oid=Marshal.StringToHGlobalAnsi("1.3.6.1.5.5.7.3.1");oidArray=Marshal.AllocHGlobal(IntPtr.Size);Marshal.WriteIntPtr(oidArray,oid);
    var parameters=new ChainParameters{Size=(uint)Marshal.SizeOf(typeof(ChainParameters)),RequestedUsage=new UsageMatch{Type=0,Usage=new EnhancedUsage{Count=1,Oids=oidArray}}};
    // Revocation is REQUIRED. Cache-only forbids network/global URL retrieval; supplied CRL is memory-only.
    // Disable AIA/root auto-update as well. No ignore flags, cache flush, time override or system clock change.
    const uint flags=0x40000000u|0x80000000u|0x00000004u|0x00000100u|0x00002000u;
    Phase="get-chain";Need(CertGetCertificateChain(engine,cert,IntPtr.Zero,other,ref parameters,flags,IntPtr.Zero,out chain));
    var prefix=(ChainPrefix)Marshal.PtrToStructure(chain,typeof(ChainPrefix));
    var ssl=new SslPolicy{Size=(uint)Marshal.SizeOf(typeof(SslPolicy)),AuthType=2,Checks=0,Name=host};
    extra=Marshal.AllocHGlobal(Marshal.SizeOf(typeof(SslPolicy)));Marshal.StructureToPtr(ssl,extra,false);
    var policy=new PolicyParameters{Size=(uint)Marshal.SizeOf(typeof(PolicyParameters)),Flags=0,Extra=extra};
    var status=new PolicyStatus{Size=(uint)Marshal.SizeOf(typeof(PolicyStatus))};
    Phase="SSL-policy";Need(CertVerifyCertificateChainPolicy(new IntPtr(4),chain,ref policy,ref status));
    return new{trusted=prefix.Trust.Error==0&&status.Error==0,chain_error=prefix.Trust.Error,ssl_policy_error=status.Error,
      memory_exclusive_roots=true,revocation_required=true,cache_only=true,system_store_provider_used=false,
      default_Framework_or_Schannel_TLS=false,real_trust_installed=false};
   }finally{
    if(extra!=IntPtr.Zero){Marshal.DestroyStructure(extra,typeof(SslPolicy));Marshal.FreeHGlobal(extra);}
    if(chain!=IntPtr.Zero)CertFreeCertificateChain(chain);if(cert!=IntPtr.Zero)CertFreeCertificateContext(cert);
    if(engine!=IntPtr.Zero)CertFreeCertificateChainEngine(engine);
    if(oidArray!=IntPtr.Zero)Marshal.FreeHGlobal(oidArray);if(oid!=IntPtr.Zero)Marshal.FreeHGlobal(oid);
    foreach(IntPtr s in new[]{roots,other,empty})if(s!=IntPtr.Zero)CertCloseStore(s,0);
   }
  }
  public static int Main(string[] args){
   try{Need(args.Length==6&&args[0]=="--check");Console.WriteLine(new JavaScriptSerializer().Serialize(Check(args[1],args[2],args[3],args[4],args[5])));return 0;}
   catch(Exception){Console.Error.WriteLine("STOP: memory-chain fixture API gate failed; phase "+Phase+" Windows code "+ApiError+" config-size "+Marshal.SizeOf(typeof(EngineConfig)));return 1;}
  }
 }
}
