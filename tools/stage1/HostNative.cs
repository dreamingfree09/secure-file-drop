using System;
using System.IO;
using System.Runtime.InteropServices;
using Microsoft.Win32.SafeHandles;
namespace Sfd.Stage1 {
 public static class HostNative {
  [DllImport("computecore.dll")] static extern IntPtr HcsCreateOperation(IntPtr c,IntPtr callback);
  [DllImport("computecore.dll",CharSet=CharSet.Unicode)] static extern int HcsEnumerateComputeSystems(string query,IntPtr operation);
  [DllImport("computecore.dll")] static extern int HcsWaitForOperationResult(IntPtr operation,uint timeout,out IntPtr result);
  [DllImport("computecore.dll")] static extern void HcsCloseOperation(IntPtr operation);
  [DllImport("kernel32.dll")] static extern IntPtr LocalFree(IntPtr p);
  [DllImport("kernel32.dll",SetLastError=true)] public static extern bool GetNamedPipeServerProcessId(SafePipeHandle pipe,out uint pid);
  [DllImport("advapi32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool ConvertStringSecurityDescriptorToSecurityDescriptor(string s,uint revision,out IntPtr sd,out uint size);
  [StructLayout(LayoutKind.Sequential)] struct SA {public int length;public IntPtr sd;public int inherit;}
  [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern bool CreateDirectory(string path,ref SA sa);
  [DllImport("kernel32.dll",CharSet=CharSet.Unicode,SetLastError=true)] static extern uint GetCompressedFileSize(string path,out uint high);
  public static ulong AllocatedBytes(string path){uint hi;uint lo=GetCompressedFileSize(path,out hi);if(lo==uint.MaxValue&&Marshal.GetLastWin32Error()!=0)throw new IOException("allocation-query");return ((ulong)hi<<32)|lo;}
  public static void SaveIso(object raw,string path){var stream=(System.Runtime.InteropServices.ComTypes.IStream)raw;IntPtr count=Marshal.AllocHGlobal(4);try{using(var file=new FileStream(path,FileMode.CreateNew,FileAccess.Write,FileShare.None)){var data=new byte[1024*1024];while(true){stream.Read(data,data.Length,count);int n=Marshal.ReadInt32(count);if(n<=0)break;file.Write(data,0,n);}file.Flush(true);}}finally{Marshal.FreeHGlobal(count);}}
  public static string Enumerate() {
   IntPtr op=HcsCreateOperation(IntPtr.Zero,IntPtr.Zero);if(op==IntPtr.Zero) throw new IOException("HCS-operation");IntPtr text=IntPtr.Zero;
   try { int result=HcsEnumerateComputeSystems("{}",op);if(result<0) Marshal.ThrowExceptionForHR(result);
    result=HcsWaitForOperationResult(op,5000,out text);if(result<0) Marshal.ThrowExceptionForHR(result);
    if(text==IntPtr.Zero) throw new IOException("HCS-empty-result");return Marshal.PtrToStringUni(text);
   }finally{if(text!=IntPtr.Zero)LocalFree(text);HcsCloseOperation(op);}
  }
  public static void NewPrivateDirectory(string path,string sid) {
   IntPtr sd=IntPtr.Zero;uint bytes;string sddl="O:"+sid+"G:"+sid+"D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;"+sid+")";
   if(!ConvertStringSecurityDescriptorToSecurityDescriptor(sddl,1,out sd,out bytes))throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());
   try {var sa=new SA{length=Marshal.SizeOf(typeof(SA)),sd=sd,inherit=0};if(!CreateDirectory(path,ref sa))throw new System.ComponentModel.Win32Exception(Marshal.GetLastWin32Error());}
   finally{LocalFree(sd);}
  }
 }
}
