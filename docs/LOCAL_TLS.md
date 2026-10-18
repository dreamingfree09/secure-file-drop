# Local Windows HTTPS and revocation — prepared correction

Work IDs: SFD-LOCAL-TLS-FINAL-CORRECTION-20261018 and SFD-OWNER-AUTONOMY-20261018. Owner: Konstantinos Aidinidis. Producer: Codex. Independent reviewer: Aether. **Prepared, not activated or accepted.** Existing live certificate and trust remain in use; normal Windows health/readiness fail revocation validation. [Exact external preparation evidence](https://github.com/dreamingfree09/secure-file-drop/pull/26#issuecomment-6245278009) distinguishes diagnostics, passed preparation checks and every unexecuted acceptance gate. This page records local development only, not deployment approval.

## Current identity and cause

Compose project securefiledrop, base /home/dreamingfree09/Secure File Drop/docker-compose.yml; proxy sfd_proxy, image traefik:v2.11, actual image sha256:bb58a3046a925d974492f2a8d12af98b44a7ed039a46d3ade1ecf470a1af96c3. Existing external override: /home/dreamingfree09/.config/secure-file-drop/local-tls-20261017-82f9b194.compose.yml. TLS namespace: /home/dreamingfree09/.local/share/secure-file-drop/local-tls/sfd-local-tls-20261017-82f9b194/. Existing read-only server/dynamic mounts and Docker socket remain; never mount the CA directory.

| Identity | Verified public value |
| --- | --- |
| Current leaf DER SHA-256 | c38479000ce9c15fdafe94d8306829a0e2acf1bd693b57be4839e8ec9f17da40 |
| Trusted CA DER SHA-256 | 0f45e72366dc8ff6e3bed9323ef697bb44b21204d7e05ea303da319b5160bc28 |
| CurrentUser Root thumbprint | A67604D69C6BFC32E77CE2933E0096C14462367C |
| Public CA PEM/file SHA-256 | 8e9e9782bc4c2705ee9b7adc60e5e6b3095b6e21dbbcf0d02a31b198bcaf30fd |

The PEM/file hash and DER fingerprint describe different encodings of the same CA. Its encrypted private key stays only in ca/root-ca.key, mode 0600 within owner-only directories. The current localhost leaf is directly issued by this trusted CA, has DNS:localhost/serverAuth and a valid signature/time period. Windows finds that root; a missing intermediate is not the diagnosis.

Normal Schannel health and readiness both return exit 35, CRYPT_E_NO_REVOCATION_CHECK (0x80092012). Strict online Windows chain building reports leaf RevocationStatusUnknown; native certutil reports no AIA/CDP URLs and unavailable leaf revocation information even though its process exits 0. Treat that diagnostic result as failure, not acceptance. The confirmed gap is usable revocation status; lack of extensions alone is not proof of every possible store/cache failure. CurrentUser CA-store inspection found no matching SFD issuer/CRL; no store was changed. No insecure flag, custom acceptance callback, policy relaxation or cache flush is permitted.

The reviewed Framework client uses normal HttpClientHandler without a certificate override. The installed Framework default CheckCertificateRevocationList was observed false; a separate defensive health/readiness check explicitly enabled it and failed. Do not change the product or count its default behavior as strict Schannel acceptance. Actual native acceptance remains separate.

## One secure Owner signing action

Preparation workspace relative to the TLS namespace: ca/revocation-20261018-5ac927e4. Its reconciled issuer ledger records the one observed original leaf serial 3E5E8272612B5CE5EB05AFC5315C4635E25E67FB, matching the untouched original CA serial file. The existing CSR signature/subject/public-key match was verified. No signed replacement leaf/CRL exists yet.

Verify the script before execution; expected SHA-256 is e8a74b2ff69260f5b6f7313409225ab1bbdcf10825bf4951dd1e9745dfbd40c8. In an interactive local WSL terminal:

~~~sh
bash '/home/dreamingfree09/.local/share/secure-file-drop/local-tls/sfd-local-tls-20261017-82f9b194/ca/revocation-20261018-5ac927e4/owner-sign.sh'
~~~

Enter the EXISTING CA passphrase only at OpenSSL's two hidden prompts. Never send it to chat/GitHub, command arguments, environment variables, plaintext files or logs. The script refuses a noninteractive channel before opening the key, locks issuance and refuses overwrite of existing output. It signs a replacement leaf from the verified existing CSR and an issuer CRL, verifies signatures and writes only a public DER CRL to revocation-public-20261018-5ac927e4/sfd-local-ca.crl. It does not change live Traefik, roots or services. If interrupted, preserve partial outputs and let Codex reconcile; do not delete them or blindly rerun. No second CA/root or unencrypted key is a substitute.

## Authorised activation and required acceptance

The prepared leaf uses DNS:localhost, CA:false, digitalSignature/keyEncipherment, serverAuth, SKI/AKI and CDP http://127.0.0.1:18081/sfd-local-ca.crl. The CRL has issuer AKI, SHA-256 signature, number and bounded dates. A loopback public HTTP CRL avoids recursive HTTPS trust bootstrapping; its signature establishes issuer authenticity. No Windows store change is planned.

After signing, Codex must verify root/leaf/CRL fingerprints, signatures, serials, ledger, issuer AKI, usages, SAN and current dates; start only the guarded endpoint; and verify Windows retrieves HTTP 200 with the exact public DER hash. The helper serves only the fixed CRL URL on WSL 127.0.0.1:18081, rejects other paths, exposes no directory/key/configuration, and refuses startup without a valid CA-signed CRL. A temporary Windows-reachable route test returned expected 503 without a CRL and 404 for other/traversal paths; the exact owned process was stopped and no listener remains. Signed-CRL delivery is not yet tested.

Reconfirm project/image/flags/routes/ports/mounts and all container IDs/start times before a live change. Preserve the existing CA/server keys, public certificates and external override. The prepared original-localhost-tls.yml, original-compose-override.yml and previous-localhost.crt are exact backups. Copy only the signed replacement public leaf to server/localhost-with-cdp-20261018-5ac927e4.crt, atomically select it through staged-localhost-tls.yml, and require its actual served fingerprint. Keep the existing key and original certificate; never infer a successful file-provider reload from file creation alone.

Mandatory Windows commands, without bypasses:

~~~powershell
curl.exe -i --max-time 5 https://localhost:8443/health
curl.exe -i --max-time 5 https://localhost:8443/ready
~~~

Both must return exit 0, HTTP 200 and expected bodies. Require strict online Windows chain/serverAuth and Framework checks with revocation enabled. Only after those pass, perform proxy-only persistence recreation with both explicit Compose files, --no-deps and --pull never; require the same intended certificate and repeated strict Windows success afterwards. Confirm unchanged backend/PostgreSQL/MinIO/Grafana/Prometheus IDs, images, start times and mounts. Never compose down, pull images or restart dependencies. Expired/revoked-CRL negative checks and actual native visual acceptance are still unexecuted, not passes.

## Future startup, shutdown and maintenance

These procedures are prepared for an accepted correction; they do not imply it has been activated. After WSL shutdown, first start the guarded endpoint:

~~~sh
python3 '/home/dreamingfree09/.local/share/secure-file-drop/local-tls/sfd-local-tls-20261017-82f9b194/ca/revocation-20261018-5ac927e4/crl-control.py' start
~~~

Then use both explicit files whenever the existing proxy requires startup:

~~~sh
docker compose -p securefiledrop \
  -f '/home/dreamingfree09/Secure File Drop/docker-compose.yml' \
  -f '/home/dreamingfree09/.config/secure-file-drop/local-tls-20261017-82f9b194.compose.yml' \
  up -d --no-deps --pull never proxy
~~~

For the authorised persistence test only, add --force-recreate after earlier strict gates pass. crl-control.py status/stop verifies owned PID/start time/UID/exact argv/script hash; it never terminates an unrelated listener. Stop only that owned helper when the local certificate environment is intentionally offline; do not stop backend/data services as routine companion shutdown. The companion's Exit closes only itself. No CRL autostart service, scheduler or automatic renewal was installed, and no continuing background agent is promised.

The Owner is the local maintenance operator: refresh the seven-day CRL before NextUpdate (target day 6), using secure local key input and the retained issuer database. Generate with openssl ca -gencrl -config openssl-ca.cnf into a NEW dated PEM, verify signature/AKI/monotonic CRL number/dates, convert to DER in a new temporary path, and atomically replace only the public CRL. Never reset the database or crlnumber (initial 0x1000). Retain issuance/revocation history. Revocation uses openssl ca -config openssl-ca.cnf -revoke EXACT_CERTIFICATE -crl_reason appropriate_reason, followed by a new signed/verified numbered CRL. Do not silently un-revoke a certificate to facilitate rollback. A cached valid CRL can permit validation until NextUpdate; endpoint outage is not evidence of immediate rejection. Missing/expired/unusable status must fail strict acceptance.

The replacement leaf is 90 days; CA expiry is 2027-10-18. Renew to a new uniquely named leaf output, preserve the existing identity/ledger and run all gates before changing selection. Maintenance is explicit, not automated passphrase storage.

## Rollback and retained gates

No live TLS change has been made by this preparation. If later activation fails acceptance, atomically restore original-localhost-tls.yml, verify the former c384...da40 fingerprint and unchanged services; if necessary recreate only proxy using both preserved Compose files, --no-deps and --pull never. Stop only the owned CRL helper through its guarded stop command. Preserve replacement/CRL/database/key material and record rejected-leaf retirement through the issuer when signing is available. Restoring the old leaf restores its known revocation failure, not readiness.

Keep the trusted CA in ordinary rollback. Remove only that exact CurrentUser root on an explicit root-retirement instruction after matching both recorded thumbprint and DER SHA-256 and proving baseline absence. Never modify LocalMachine/unrelated roots. Returning all the way to original generated Traefik TLS additionally omits the external override from the proxy-only invocation and restores the original trust failure; its ephemeral fingerprint may differ.

The existing WSL feature environment at 2b8e56de4b79e2508125ca02c18455ae88dee7f6 is not canonical M24; actual backend-image provenance/API compatibility is unverified by TLS checks. Preserve its backend/data while resolving that distinction before daily-use acceptance. All fifteen [Windows operator checks](WINDOWS_COMPANION.md#operator-acceptance-checklist--final-review-2026-10-17), including interactive Explorer opt-in, remain outstanding. Existing all-interface exposure, archived storage/security maintenance, backend-storage HTTP, capacity, rotation, topology, historical reconciliation, combined cross-host recovery and public-production gates remain open. Local HTTPS does not clear them.
