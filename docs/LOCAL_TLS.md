# Local Windows HTTPS and revocation — active local correction

SFD-LOCAL-TLS-FINAL-CORRECTION-20261018, under SFD-OWNER-AUTONOMY-20261018. Owner: Konstantinos Aidinidis. Producer: Codex. Independent reviewer: Aether. **Automated local TLS and persistence acceptance passed; actual Windows interactive acceptance and Aether review remain pending.** This is local development evidence, not production readiness. The [exact external handover](https://github.com/dreamingfree09/secure-file-drop/pull/26#issuecomment-6245278009) records public identities, commands, preservation and limitations. The earlier prepared-only state remains in Git history.

## Current identity

Compose project securefiledrop; base /home/dreamingfree09/Secure File Drop/docker-compose.yml; unchanged external override /home/dreamingfree09/.config/secure-file-drop/local-tls-20261017-82f9b194.compose.yml. TLS namespace /home/dreamingfree09/.local/share/secure-file-drop/local-tls/sfd-local-tls-20261017-82f9b194/; issuer workspace ca/revocation-20261018-5ac927e4 beneath it.

| Public identity | Verified value |
| --- | --- |
| Active leaf DER SHA-256 | 4106ecd1f0bd2d94be7a7fcf2236cb9b0119dde5aa82a97edd2cdab584809b70 |
| Active leaf serial | 8C6395BB851AF8FE0E80DA3C71763916 |
| Leaf validity, UTC | 2026-10-18T12:17:28Z through 2027-01-16T12:17:28Z |
| Preserved previous leaf DER SHA-256 | c38479000ce9c15fdafe94d8306829a0e2acf1bd693b57be4839e8ec9f17da40 |
| Trusted CA DER SHA-256 | 0f45e72366dc8ff6e3bed9323ef697bb44b21204d7e05ea303da319b5160bc28 |
| CurrentUser Root thumbprint | A67604D69C6BFC32E77CE2933E0096C14462367C |
| Public CA PEM/file SHA-256 | 8e9e9782bc4c2705ee9b7adc60e5e6b3095b6e21dbbcf0d02a31b198bcaf30fd |
| Public DER CRL SHA-256 | d5e8ca59b3324fe4f133d2c381737bab413340085d990469d1691a9688a6b5cc |
| CRL number | 0x1000; issuer ledger's next number 0x1001 |
| CRL validity, UTC | 2026-10-18T12:17:40Z through 2026-10-25T12:17:40Z |
| CRL URL | http://127.0.0.1:18081/sfd-local-ca.crl |

PEM/file and DER fingerprints describe different encodings of the same verified CA. Existing encrypted CA key ca/root-ca.key, server key server/localhost.key and root-ca.srl were preserved. No private key/passphrase was printed, exported or copied. CurrentUser/LocalMachine Root inventories remain 80/79 with zero additions/removals during this correction. No extra root or CRL-store import was needed. Normal Windows retrieval caches public revocation data; no cache flush or policy change was made.

## Applied correction and actual validation

Before correction, normal Schannel health/readiness failed exit 35, CRYPT_E_NO_REVOCATION_CHECK. Windows found the intended root but could not obtain usable leaf revocation status; the old leaf lacked CDP/AIA. Native certutil's process exit 0 did not override its explicit revocation failure. No missing intermediate was diagnosed. The root already had SKI and CRL-signing usage.

The Owner executed the verified owner-sign.sh once, entering the existing CA passphrase only at OpenSSL's two hidden prompts. Its SHA-256 remains e8a74b2ff69260f5b6f7313409225ab1bbdcf10825bf4951dd1e9745dfbd40c8. Do not rerun it over existing outputs; its collision guards preserve them. Future maintenance uses new output paths as below. The known previous issuance was reconciled in index.txt, matching the original serial state; the new valid issuance and monotonic CRL number are retained. No issued certificate was revoked solely for a test.

The replacement uses the existing verified CSR/server key, DNS:localhost, critical CA:false and digitalSignature/keyEncipherment, serverAuth, SKI/issuer AKI and the loopback CDP above. Leaf/CRL signatures, dates, serial/number, issuer and matching AKI verified against the unchanged CA. Only the new public leaf was copied to server/localhost-with-cdp-20261018-5ac927e4.crt. Dynamic localhost-tls.yml was atomically switched to it; the original public certificate/key and backed-up configuration remain intact. The override, original repository Compose file, image, routes, ports and read-only socket/server/dynamic mounts were preserved. CA material is not mounted into Docker.

The guarded crl-control.py endpoint is active, bound only to WSL 127.0.0.1:18081 and forwarded on Windows 127.0.0.1. It serves only the fixed public CRL path, no directory/key/configuration. Startup verifies the current CA-signed DER CRL and its validity; missing material refuses startup. Windows fetched HTTP 200 and matched the exact public DER hash. Preparation tests returned expected 503 for an absent CRL and 404 for other/traversal paths. The earlier temporary test process was stopped after identity checks; the accepted helper is separately recorded in the handover. No autostart service or scheduler was installed.

**Passed before and after proxy-only recreation:** actual SNI localhost certificate fingerprint above; verified hostname/signature; strict online Windows X509Chain with serverAuth and NoFlag (no errors); normal Schannel curl health/readiness, both exit 0 and HTTP 200 with JSON status ok; .NET Framework health/readiness with revocation explicitly enabled, both HTTP 200. Unauthenticated GET /me returned 401 with normal TLS and no credentials/cookies, confirming a protected route, not full authenticated API compatibility.

~~~powershell
curl.exe -i --max-time 5 https://localhost:8443/health
curl.exe -i --max-time 5 https://localhost:8443/ready
~~~

Persistence recreation used both explicit Compose files, project securefiledrop, up -d --no-deps --pull never --force-recreate proxy. Old proxy fd468c8a0cbeec643d3e1f38126c6135bd08dd973d9b6facec86451b4ebb17ea was replaced by 7d1e8f7fc97a00d8d41ea3f783458c6589893cabc4df64b7d84f7dcfa552dcd7, started 2026-10-18T12:20:53.614357972Z. Image remains sha256:bb58a3046a925d974492f2a8d12af98b44a7ed039a46d3ade1ecf470a1af96c3 (traefik:v2.11); same arguments, ports and mounts. All five non-proxy IDs/images/start times/mounts compare unchanged. No database/storage/volume operation or dependent restart.

Bounded offline negative checks rejected a missing CRL, the authentic signed CRL one second after NextUpdate (test-time parameter only), and a corrupted public CRL signature; live CRL, Windows clock/cache/policy and issuer ledger were unchanged. Deliberate actual-leaf revocation and Windows revoked/expired-CRL rejection were not executed; do not count offline negatives as those runtime tests. The reviewed Framework handler's existing default revocation property is false; the separate strict checks explicitly enabled it and passed. No product callback/default/security control was weakened.

## Startup, shutdown and maintenance

After WSL shutdown, first start the guarded endpoint:

~~~sh
python3 '/home/dreamingfree09/.local/share/secure-file-drop/local-tls/sfd-local-tls-20261017-82f9b194/ca/revocation-20261018-5ac927e4/crl-control.py' start
~~~

Then use both preserved files whenever the existing proxy requires startup:

~~~sh
docker compose -p securefiledrop \
  -f '/home/dreamingfree09/Secure File Drop/docker-compose.yml' \
  -f '/home/dreamingfree09/.config/secure-file-drop/local-tls-20261017-82f9b194.compose.yml' \
  up -d --no-deps --pull never proxy
~~~

crl-control.py status/stop verifies UID, PID/start ticks, exact argv and script hash; never stop an unrelated listener. Stop only this helper when the local TLS environment is intentionally offline. Companion Exit closes only the companion, not backend/Docker/WSL. No continuing background agent is promised. Do not compose down, pull images or restart dependencies for companion use.

**Owner maintenance:** refresh before **2026-10-25T12:17:40Z**, target day 6 (2026-10-24), and before each subsequent seven-day NextUpdate. Use local hidden key input and the existing issuer ledger. From that workspace, generate openssl ca -gencrl -config openssl-ca.cnf into a NEW dated PEM; verify issuer/signature/AKI/monotonic CRL number/dates; convert to DER in a new temporary path; atomically replace only revocation-public-20261018-5ac927e4/sfd-local-ca.crl. Keep records and crlnumber; do not recreate/reset the database. No automatic refresh/renewal or stored passphrase is claimed. A cached valid CRL may permit validation until NextUpdate; endpoint outage is not proof of immediate rejection. Unavailable/expired/unusable status must not be softened into acceptance.

Actual revocation uses openssl ca -config openssl-ca.cnf -revoke EXACT_CERTIFICATE -crl_reason appropriate_reason, followed by a newly signed/verified numbered CRL. Preserve certificate/ledger evidence first; do not silently un-revoke for rollback. Renewal uses a new unique output path and all current localhost extensions; verify full strict TLS/persistence gates before selection. The active leaf expires 2027-01-16T12:17:28Z; CA expires 2027-10-18T02:59:17Z. Owner secret input remains necessary for protected signing operations.

## Prepared rollback and remaining acceptance

Rollback was not invoked. If required by failed acceptance or explicit recovery, atomically restore issuer-workspace original-localhost-tls.yml, confirm the old c384...da40 fingerprint and unchanged services; if recreation is necessary, use both preserved Compose files with project securefiledrop, --no-deps and --pull never, proxy only. Stop only the owned helper through guarded crl-control.py stop. Preserve all old/new certificates, CRLs, database and keys; record rejected replacement retirement through the issuer when signing is available. Restoring the old leaf intentionally restores its known revocation failure, not readiness.

Ordinary rollback retains the CA root. Root retirement requires explicit instruction and matching BOTH the recorded CurrentUser thumbprint and DER SHA-256 plus original baseline absence. Never alter LocalMachine or unrelated roots. Full original-generated-Traefik recovery additionally omits the external override from the proxy-only invocation and restores original trust failure; an ephemeral fallback fingerprint can differ. No key deletion or broad teardown.

The existing WSL feature checkout at 2b8e56de4b79e2508125ca02c18455ae88dee7f6 is not canonical M24, and backend source/revision labels are empty. Health/ready and unauthenticated /me do not establish image provenance or full authenticated API compatibility. Preserve existing backend/data while completing that distinction and the actual [Windows operator checklist](WINDOWS_COMPANION.md#operator-acceptance-checklist--final-review-2026-10-17). A pre-existing companion process was left untouched; its on-disk executable hash matches the reviewed preview even though it runs from another path. The Owner can use that existing tray instance for visual checks. No automatic launch, credential/settings read, Explorer registration or clipboard operation occurred. All actual native visual/authenticated journeys and Aether exact-revision acceptance remain pending. Existing all-interface exposure, archived-storage/security maintenance, backend-storage HTTP, capacity, rotation, topology, historical reconciliation, combined cross-host recovery and public deployment gates remain open.
