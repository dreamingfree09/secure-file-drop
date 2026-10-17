# Windows tray companion — development preview

Work ID: SFD-OWNER-DELEGATED-COMPLETION-20261017. The Owner authorised this companion after bounded core acceptance at M24, 8d54031c26ec10027ecebadd754f34996d55e8dd. Producer: Codex. Independent security-sensitive review and integration: Aether, pending. This is a Windows client for the existing Go backend and web UI. It neither runs nor reconfigures Docker, WSL, the backend or storage.

## Build and run

Use a supported Windows environment with .NET Framework 4.8. No embedded browser, external desktop framework or NuGet runtime package is included. The existing Windows compiler builds the application; scripts do not install prerequisites.

~~~powershell
.\desktop\windows\build.ps1
.\desktop\windows\dist\SecureFileDrop.exe
~~~

Ship SecureFileDrop.exe together with SecureFileDrop.exe.config. The application manifest requests the current user's privileges. The preview is unsigned; packaging/signing and distribution acceptance are separate release work. Keep the executable in its chosen location before enabling Explorer integration.

Open **Settings / Explorer integration** from the tray. Set the backend's full HTTPS origin, such as https://sfd.example.test:8443, without a path, credentials, query or fragment. The default is https://localhost:8443; remote/Proxmox-hosted services are supported through configuration. No internet exposure is created. HTTP is an explicit isolated-development option restricted to literal loopback IPs; HTTPS links remain required for sharing.

Match the companion's file limit to the configured backend/proxy. Sign in using an existing registered account; registration remains in the web interface. Passwords are never saved. Optionally remember the session using Windows current-user DPAPI. If secure storage fails, the UI explains that the session is only active for the current run.

## Tray and Explorer actions

- **Quick Send** opens the native file picker and streams one selected file through authenticated POST /files and POST /upload.
- **Open Secure File Drop** opens the configured web interface.
- **Copy Download Link** requests a fresh five-minute signed link only after size and SHA-256 match the server's native-hash metadata. The link must be HTTPS, use the exact configured authority and /download route, and have a current bounded expiry.
- **Transfer status** opens the progress view. Byte progress describes bytes sent to HTTP; completion waits for server confirmation and independent local/server SHA-256 equality.
- **Check last transfer** reconciles a known uncertain upload through the account-filtered recent-file list; it never automatically resends or deletes bytes.
- **Service status** probes readiness independently of sign-in.
- **Sign out** revokes the current server session and removes its protected local copy. Network/revocation failures remain actionable, rather than claiming server revocation succeeded.
- **Exit** cancels the companion's active operation and removes its tray icon. It never terminates the backend, Docker, WSL or unrelated processes.

Explorer integration is opt-in through Settings, scoped to HKCU\Software\Classes\*\shell\SecureFileDrop.Send and does not require administrator rights. The static verb appears under **Show more options** on Windows 11. The command quotes the full executable and selected file path, invokes the executable directly, and never invokes a command shell. A current-user/session-restricted named pipe forwards later invocations to the existing tray instance. An unrelated registry entry is preserved. Removal checks the companion ownership marker.

Only one transfer/account operation runs at a time; a second file is rejected with a busy indication, not silently queued. Local drive paths with spaces and Unicode are supported. Direct network/device/ADS arguments, directories and reparse-point file placeholders are rejected. The existing backend's filename contract (255 UTF-8 bytes, no controls, basename normalization) remains authoritative.

## Authentication, confidentiality and TLS

The client uses the backend's standard sfd_session HttpOnly cookie. HTTPS cookies must be Secure. It verifies GET /me, handles 401 expiry/revocation by clearing authentication, and clears previous-account transfer/link state when signing in again. Existing custom cookie-name deployments are outside this initial client's default-cookie contract.

A remembered session is encrypted with DPAPI CurrentUser and bound to the exact backend origin in both its protected content and its hashed filename. Only session tokens/expiry are remembered; passwords, signing/admin secrets, file contents and signed download URLs are never written to application files or logs. Current-user DPAPI is a Windows account boundary; it does not defend against malicious code already running as that user.

HTTP redirects are rejected for all API calls, preventing password/cookie/file-body forwarding. Normal OS HTTPS certificate validation remains enabled; there is no accept-all callback or insecure-TLS option. Backend settings accept only an origin and prevent credentials, query strings, fragments or arbitrary launch schemes. Browser launch receives only the validated origin. Explorer paths use argument parsing and FileStream, never shell evaluation.

Copied bearer links live only in memory and the clipboard. The companion clears its own clipboard value after two minutes or sign-out/exit if it is still unchanged. Other applications, clipboard history/cloud sync and recipients can retain a copied link; treat it as sensitive until expiry. Notifications show completion/errors without a token or URL. An origin change clears previous-account state and attempts revocation before changing configuration.

## Large files and interruption

Upload memory is bounded by a 64 KiB file buffer plus bounded metadata/multipart headers. File bytes are streamed, hashed incrementally and checked against the server metadata; the entire file is not loaded into RAM. Windows read sharing prevents ordinary concurrent writers during upload. A 64 MiB local counting-sink test exercises this streaming mechanism, not backend capacity.

Existing backend constraints remain unchanged:

| Constraint | Existing behavior / demonstrated boundary |
| --- | --- |
| Development Compose limit | 10 MiB file bytes; 11 MiB total request envelope |
| Backend without an explicit file limit | 50 GiB ceiling, not demonstrated large-file capacity |
| Upload budget | Five minutes total, with native hashing at most two minutes within it |
| Demonstrated migration objects | 0, 38 and 5 MiB + 17-byte objects, including explicit multipart completion |
| New real proxy/client contract | 1,024-byte exact-limit Unicode upload, empty file, actual HTTPS byte-equal download |
| Resume | No resumable-upload protocol; no automatic replay of an uncertain upload |

Raising the companion limit cannot raise proxy/backend limits or extend server deadlines. Representative multi-GiB, slow-client, throughput and storage-capacity evidence is outstanding. This initial companion makes the existing bounded upload convenient; it does not certify arbitrary large-file reliability.

Cancel stops the request. A lost response can still mean a committed server upload. **Check last transfer** verifies the existing record and file hash before enabling a link; pending/missing/failed or inaccessible records require web inspection and normal server retention. Session/account changes deliberately clear stale transfer state, so use the web interface to inspect older uncertain attempts after signing in again. No automatic compensation, deletion or resend occurs.

## Validation and review gates

~~~powershell
.\desktop\windows\tests\test.ps1
~~~

Actual local Windows execution passed 32 platform/security/lifecycle checks and 10 HTTP protocol checks: current-user DPAPI and tamper isolation; quoted Unicode/metacharacter argument roundtrip; owned registry logic/unowned-entry preservation; current-user/session pipe; 64 MiB bounded writes and cancellation; real NotifyIcon/menu instantiation and exit; cookie login; Unicode multipart; 401 handling; in-flight cancellation with committed-response reconciliation; redirect isolation; unavailable backend. Registry tests use fresh non-Explorer test keys, session/files use owned temporary directories, and the UI readiness fixture is a fresh owned loopback listener. The real user's Explorer registration and application session directory are untouched.

A portable .NET 10 test harness compiles the exact Client.cs against the existing real Traefik/backend/PostgreSQL/MinIO/native fixture. It is opt-in with SFD_DESKTOP_CLIENT=1 and uses SSL_CERT_FILE only in that child process to trust the fresh fixture CA. Normal client certificate validation remains unchanged, and a second untrusted TLS fixture must receive zero application requests. No machine certificate trust is modified. Hosted execution results and exact published revisions belong in [PR #26](https://github.com/dreamingfree09/secure-file-drop/pull/26); build evidence alone is not a runtime pass.

A first local .NET SDK invocation automatically generated an untrusted localhost development certificate. Its exact creation time/identity were verified and only that test-created certificate/private key was removed; no existing certificates or trusted roots were changed. Subsequent test builds disable SDK certificate generation and telemetry.

**Unexecuted interactive acceptance:** visual notification-area placement; native file-picker interaction; an actual Explorer user right-click/invocation; real-user-profile first launch; clipboard UI verification; and the complete Windows Framework client journey against a real HTTPS deployment. Programmatic tray-context/menu creation/exit, owned registry/argument tests and hosted portable real-backend execution are distinct evidence. Aether should review the exact proposal, and a Windows operator should complete these visible journeys before declaring the companion fully accepted or installing it for daily use.

## Preserved production boundaries

Default storage, Go application/native code, schema, root dependencies and backend APIs remain unchanged by the companion. The new opt-in test hook preserves existing assertions. The exact-M24 workflow continues to check out unchanged M24.

Archived MinIO remains restricted to trusted isolated synthetic use; its runtime advisory inventory is not cleared by functional acceptance. Production storage/vendor/security-update ownership, backend-storage TLS, capacity, topology/maturity, credential rotation, historical schema/ownership reconciliation, combined cross-host DR, real-data migration/cutover and public deployment gates remain open. See [continuity](CONTINUITY.md), [deployment](DEPLOYMENT.md), [tracker](TRACKER.md) and the PR's exact-revision GHP-01 handover.

Architecture references: Microsoft [NotifyIcon](https://learn.microsoft.com/en-au/dotnet/desktop/winforms/controls/notifyicon-component-windows-forms?view=netframeworkdesktop-4.8), [current-user data protection](https://learn.microsoft.com/en-us/dotnet/api/system.security.cryptography.dataprotectionscope), [Shell registration](https://learn.microsoft.com/en-us/windows/win32/shell/app-registration) and [redirect behavior](https://learn.microsoft.com/dotnet/api/system.net.http.httpclienthandler.allowautoredirect).
