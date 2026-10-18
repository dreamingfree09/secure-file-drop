// The standalone csc build must declare its target, not just its CLR activation version.
// This opts .NET Framework into OS-selected TLS and strong-crypto defaults.
[assembly: System.Runtime.Versioning.TargetFramework(".NETFramework,Version=v4.8", FrameworkDisplayName = ".NET Framework 4.8")]
