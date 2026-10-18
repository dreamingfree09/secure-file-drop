# Windows tray companion — development preview

Work ID: SFD-OWNER-DELEGATED-COMPLETION-20261017. The Owner authorised this companion after bounded core acceptance at M24, 8d54031c26ec10027ecebadd754f34996d55e8dd. Producer: Codex. Independent security-sensitive review and integration: Aether, pending. This is a Windows client for the existing Go backend and web UI. It neither runs nor reconfigures Docker, WSL, the backend or storage.

## Local operator prerequisite

Follow the [local TLS and revocation runbook](LOCAL_TLS.md) before native Windows acceptance. The existing preview/source identity is unchanged by that external configuration work. Strict Schannel health/readiness and persistence are not yet accepted; secure Owner signing and the subsequent automated gates are pending. Do not count an OpenSSL-only or default-handler connection as those strict gates. The live historical WSL backend is not proven to be the accepted M24 runtime; confirm API/source compatibility before daily-use acceptance without changing existing backend/data implicitly. Standing execution authority is recorded in [GHP-01](GHP-01.md#standing-owner-delegation--2026-10-18); it does not satisfy the unexecuted visual checklist or Aether review.

## Build and run

Use a supported Windows environment with .NET Framework 4.8. No embedded browser, external desktop framework or NuGet runtime package is included. The existing Windows compiler builds the application; scripts do not install prerequisites.

~~~powershell
.\desktop\windows\build.ps1
.\desktop\windows\dist\SecureFileDrop.exe
~~~

Ship SecureFileDrop.exe together with SecureFileDrop.exe.config. The application manifest requests the current user's privileges. The preview is unsigned; packaging/signing and distribution acceptance are separate release work. Keep the executable in its chosen location before enabling Explorer integration.

Open **Settings / Explorer integration** from the tray. Set the backend's full HTTPS origin, such as https://sfd.example.test:8443, without a path, credentials, query or fragment. The default is https://localhost:8443; remote/Proxmox-hosted services are supported through configuration. No internet exposure is created. The shipping client requires HTTPS for local and remote services. Certificate failures remain failures; no UI/configuration option enables plaintext HTTP. Plain HTTP exists only in explicitly compiled, owned synthetic test fixtures and is absent from the release client.

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

Progress callbacks are bound to their initiating operation, and readiness replies to their initiating backend client. Late results cannot update a newer transfer or configuration. An incomplete sign-out warning remains visible after saving a new configuration. An old-backend logout timeout permits switching to the new address with that warning; explicit user cancellation still aborts the change. A remembered but unverified session is also revoked/cleared on reconfiguration.

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

Actual local Windows execution passed 32 platform/security/lifecycle checks, 14 HTTP protocol checks and three separately compiled release-policy checks: current-user DPAPI and tamper isolation; quoted Unicode/metacharacter argument roundtrip; owned registry logic/unowned-entry preservation; current-user/session pipe; 64 MiB bounded writes and cancellation; real NotifyIcon/menu instantiation and exit; cookie login; Unicode multipart; 401 handling; in-flight cancellation with committed-response reconciliation; redirect isolation; unavailable backend. Registry tests use fresh non-Explorer test keys, session/files use owned temporary directories, and the UI readiness fixture is a fresh owned loopback listener. The real user's Explorer registration and application session directory are untouched.

Synthetic Windows protocol/readiness fixtures and the portable contract harness define SFD_SYNTHETIC_TESTS; only those builds contain the internal literal-loopback HTTP fixture capability. It is not serialized or exposed in the UI. The release is compiled without that symbol; separate checks verify HTTPS success, rejection of configuration-injected HTTP flags and absence of the capability from the release type.

A portable .NET 10 test harness compiles the exact Client.cs against the existing real Traefik/backend/PostgreSQL/MinIO/native fixture. It is opt-in with SFD_DESKTOP_CLIENT=1 and uses SSL_CERT_FILE only in that child process to trust the fresh fixture CA. Normal client certificate validation remains unchanged, and a second untrusted TLS fixture must receive zero application requests. No machine certificate trust is modified. Hosted execution results and exact published revisions belong in [PR #26](https://github.com/dreamingfree09/secure-file-drop/pull/26); build evidence alone is not a runtime pass.

A first local .NET SDK invocation automatically generated an untrusted localhost development certificate. Its exact creation time/identity were verified and only that test-created certificate/private key was removed; no existing certificates or trusted roots were changed. Subsequent test builds disable SDK certificate generation and telemetry.

**Unexecuted interactive acceptance:** visual notification-area placement; native file-picker interaction; an actual Explorer user right-click/invocation; real-user-profile first launch; clipboard UI verification; and the complete Windows Framework client journey against a real HTTPS deployment. Programmatic tray-context/menu creation/exit, owned registry/argument tests and hosted portable real-backend execution are distinct evidence. Aether should review the exact proposal, and a Windows operator should complete these visible journeys before declaring the companion fully accepted or installing it for daily use.

## Operator acceptance checklist — final review 2026-10-17

Work ID: SFD-DESKTOP-FINAL-REVIEW-20261017. **Prepared, not executed.** Aether's independent acceptance and GHP-01 integration remain pending. Use an existing authorised synthetic HTTPS deployment and ordinary test accounts/files only. This checklist does not authorise provisioning a service, using real data, changing trust/security settings or installing the companion automatically.

Candidate: [unsigned preview from run 38988242982](https://github.com/dreamingfree09/secure-file-drop/actions/runs/38988242982/artifacts/12083732517), source 8f55c9deb24b477ecfc516bb67ee8078bb9ad0f8. The final-review commit changes documentation only; this artifact retains its actual build revision. Artifact expires 2026-10-31 at 00:51:27 UTC. After expiry, obtain a fresh reviewed build and verify its own revision/hash; do not treat a replacement as this candidate.

1. **Record the candidate.** Download from the linked GitHub run and extract into a new operator-chosen test folder. Keep all seven files together, including configuration, MIT licence, README, identity, results and checksums. Run `Get-FileHash -LiteralPath .\SecureFileDrop.exe -Algorithm SHA256` from that folder. Require **97fc2dafbc469627f9ee90896903162c8804c01a167402f414d150857a0ed394** and configuration SHA-256 **4d83c0772f4b593d6e16657fb5be7e4478321cb9782ba744c93b90915fb7175b**. Confirm the identity names source 8f55c9d, .NET Framework 4.8 and unsigned preview. Stop on mismatch. A checksum establishes identity, not publisher signing or release approval.
2. **Prepare safely.** Record Windows edition/build and existing Framework version. Require Framework 4.8 already available. Use an authorised test profile without existing companion settings; if existing application data is present, preserve it and arrange a separate test profile. Do not delete it for a fresh-start result. Do not disable SmartScreen, antivirus or certificate checks. If policy blocks this unsigned candidate, record blocked and stop; use the normal separately approved release/signing process.
3. **First launch and tray.** Start the executable manually, without elevation, autostart or an installer. Confirm one shield tray icon, including the notification-area overflow, and a usable right-click menu. First launch should show Settings after its initial service check; allow its five-second readiness budget. Confirm Quick Send, Open Secure File Drop, Copy Download Link, Check last transfer, service/transfer status, Sign in/out, Settings and Exit are present. A missing icon or inaccessible controls fails this step.
4. **Settings and TLS.** Enter the existing test backend's HTTPS origin with correct externally visible port and no path/query/credentials. Match the tested backend limit; keep the existing 10 MiB development limit unless the operator has another already-tested limit. Reject HTTP, credential-bearing and path-bearing settings. Check the agreed pre-existing invalid-certificate test origin: service remains unavailable and sign-in/upload cannot succeed. Do not install certificates, ignore a warning or change Windows trust. Restore the valid origin and confirm readiness recovers. If no approved invalid-certificate fixture exists, record this case unexecuted rather than altering the machine.
5. **Service and web status.** With the valid backend, allow the next 30-second readiness refresh and confirm ready. Open Secure File Drop and check that only the configured HTTPS origin launches in the normal browser. Use an agreed unavailable test origin to verify unavailable status, then restore the valid origin. Do not stop or reconfigure any service to simulate failure. Readiness does not imply authentication, storage security or production approval.
6. **Authentication.** Try one incorrect password, then sign in with an ordinary synthetic account; ensure failure gives a useful message and success enables account actions. Passwords must not appear in notifications or saved settings. Leave Remember disabled initially, Exit and relaunch: require sign-in again. Separately opt into Remember, sign in, Exit/relaunch and confirm valid-session restoration. Confirm another backend origin does not inherit that session. These are real Windows Framework checks, not portable-contract evidence.
7. **Quick Send and picker.** Send one synthetic local regular file through the native picker, with spaces and Unicode in its name, then an empty file. Confirm one file is selected, picker cancellation sends nothing, and progress opens for an actual send. Reject a directory, direct network/device or alternate-stream argument without reading external data. Keep all test files within the agreed size/filename limits.
8. **Progress and integrity.** Send a synthetic file large enough to see progress within the existing limit. Observe byte progress, then the waiting-for-server-hash state; completion must follow size/SHA-256 confirmation. Download its new signed link through the authorised browser and compare byte count and `Get-FileHash` with the original. Do not infer multi-GiB, slow-client or resumable support from this result.
9. **Cancel and uncertain outcomes.** Cancel while a transfer is active. Confirm no automatic resend/deletion and an actionable status. Use Check last transfer: an already committed matching file can become confirmed; pending/missing/unavailable records must remain uncertain and require web inspection. Do not retry until its state is understood. A naturally too-fast transfer may complete before cancellation; record that outcome and repeat only with another bounded synthetic file. Hiding the progress window must leave tray/status usable.
10. **Single instance and busy state.** While an operation is active, attempt a second bounded file invocation; confirm a busy indication and no silent second upload. Relaunch the executable and confirm it forwards to the one existing tray instance. No unrelated process should be closed.
11. **Explorer, only with the operator's explicit opt-in.** Choose a stable test-folder location first, then enable the per-user action in Settings. Right-click a local synthetic Unicode/spaces file; on Windows 11 use Show more options. Confirm Send with Secure File Drop reaches the existing instance and the exact selected file, with no command shell or elevation. Remove the action through Settings and confirm it disappears. If an unrelated entry blocks registration, preserve it and record blocked; do not overwrite or manually delete it. This is the only planned real Explorer registration, performed by the operator, not by this review.
12. **Clipboard and link expiry.** After a confirmed upload, Copy Download Link should place a current five-minute HTTPS link for the configured authority on the clipboard. Check the agreed browser download, without copying the bearer URL into an acceptance report or screenshot. Leave its clipboard value unchanged for at least two minutes plus one 30-second status interval and confirm it clears. Repeat, replace it with harmless text and confirm the companion preserves that newer text. Check expiry through the browser after the link's stated lifetime. Clipboard history/cloud sync may retain copies outside the app; do not change Windows settings as part of this test.
13. **Logout, expiry and account isolation.** Sign out and confirm the current session is revoked, its protected copy removed, the companion's unchanged clipboard link cleared and upload/link actions require sign-in. Relaunch and confirm the revoked remembered session is not restored. For an already-expired/revoked test session, confirm 401 handling clears authentication. Sign in as another ordinary account and confirm the previous transfer/link state cannot be copied. Do not bypass authentication to test this.
14. **Backend changes.** Change to another already-authorised HTTPS test origin and confirm old-account/transfer/clipboard state is cleared and the new service requires its own sign-in. If old-service logout cannot be confirmed, require the retained warning; explicit user cancellation must abort the settings operation. Do not claim successful revocation when the old service is unavailable.
15. **Clean exit and record.** Exit idle, then separately choose cancellation-and-exit during a synthetic active operation. Confirm the tray icon and companion process close, its unchanged clipboard value clears, and backend/Docker/WSL/unrelated processes continue. Inspect uncertain uploads through the web interface before any retry. Leave existing user data untouched; removal of the owned Explorer action is through its explicit app control only.

Record operator/date, Windows/Framework versions, candidate source/hash, test backend identifier (no credentials), synthetic file sizes/hashes and each numbered result as passed, failed, blocked or unexecuted. Include any error text without tokens/passwords/private paths. All fifteen steps currently remain **unexecuted interactive acceptance**. Send the completed record through the existing PR #26 review process. Daily-use acceptance needs the full native Windows HTTPS journey and Aether's exact-revision acknowledgement; passing portable or programmatic tests alone does not satisfy it.

## Preserved production boundaries

Default storage, Go application/native code, schema, root dependencies and backend APIs remain unchanged by the companion. The new opt-in test hook preserves existing assertions. The exact-M24 workflow continues to check out unchanged M24.

Archived MinIO remains restricted to trusted isolated synthetic use; its runtime advisory inventory is not cleared by functional acceptance. Production storage/vendor/security-update ownership, backend-storage TLS, capacity, topology/maturity, credential rotation, historical schema/ownership reconciliation, combined cross-host DR, real-data migration/cutover and public deployment gates remain open. See [continuity](CONTINUITY.md), [deployment](DEPLOYMENT.md), [tracker](TRACKER.md) and the PR's exact-revision GHP-01 handover.

Architecture references: Microsoft [NotifyIcon](https://learn.microsoft.com/en-au/dotnet/desktop/winforms/controls/notifyicon-component-windows-forms?view=netframeworkdesktop-4.8), [current-user data protection](https://learn.microsoft.com/en-us/dotnet/api/system.security.cryptography.dataprotectionscope), [Shell registration](https://learn.microsoft.com/en-us/windows/win32/shell/app-registration) and [redirect behavior](https://learn.microsoft.com/dotnet/api/system.net.http.httpclienthandler.allowautoredirect).
