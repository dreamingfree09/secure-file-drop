# Rorschach private deployment candidate

Issue [#55](https://github.com/dreamingfree09/secure-file-drop/issues/55) tracks the release. R1 is a dedicated Linux VM deployment candidate. It uses the existing application and native C hash contract with a 10 MiB file cap. The legacy resumable route remains disabled. Public ingress, a 100 GiB transfer, local account approval and manual protected sharing have separate acceptance gates.

## Runtime and containment

Use a freshly inventoried Debian or Ubuntu amd64 VM on Proxmox with its own supported Docker Engine (28 or later, including the loopback publication fixes), Compose plugin (2.30 or later), Python 3.11+, Git and systemd. Keep the VM off shared host directories, privileged guest mappings and existing Docker data roots. Confirm private management reachability, disk ownership and current available space before provisioning. Verify the source, guest packages and container digests before starting. Do not copy the Railway environment or the development stack.

| Component | Candidate | Isolation |
| --- | --- | --- |
| Backend | Exact reviewed source, existing Go 1.26.10 and native C build with immutable base manifests | Non-root, 512 MiB memory limit, read-only root, bounded hash scratch, private networks |
| PostgreSQL | 16.15 Alpine, linux/amd64 manifest in Compose | Private database bridge, persisted volume, dedicated non-superuser application role |
| S3 | RustFS 1.0.4, source commit 46337942d55f3a8e6a97b147ca98d9ea8115555d, linux/amd64 manifest in Compose | Private object bridge, native verified TLS, no alternate protocol listeners or published storage port |
| Proxy | Caddy 2.11.9 Alpine, linux/amd64 manifest in Compose | Host loopback HTTPS only, admin API and certificate automation disabled, no access-log directive, no Docker socket |
| One-shot provisioner | Official RustFS rc v0.1.37, release archive SHA-256 pinned at build | Restricted private object bridge, ephemeral filesystem, no host storage listener |

Both SeaweedFS and RustFS are Apache-2.0 projects. See the repository's [comparison and recovery evidence](STORAGE_EVALUATION_20261016.md) for earlier evaluations of SeaweedFS 4.48 and RustFS 1.0.2. The SeaweedFS fixture required coordinated master, volume and filer recovery, and that tested image had unresolved dependency findings. Its newer 4.52 is a viable comparator with no current R1 runtime evidence. RustFS 1.0.4 is the first candidate because it follows the successful earlier scoped IAM and recovery work. The tag, image, CLI and hosted results in this proposal are separate from vendor approval, a dependency audit or a production support decision. Single-node single-disk storage has no redundancy. See the [RustFS release](https://github.com/rustfs/rustfs/releases/tag/1.0.4), [advisories](https://github.com/rustfs/rustfs/security/advisories), and [single-node guidance](https://docs.rustfs.com/en/installation/container).

The application connects to S3 by path style and verifies its certificate against a dedicated CA. It never receives the root storage identity. The bootstrap provisions a bucket-scoped identity using the existing IAM policy template, then checks signed read/write, five anonymous 403 responses and exact readback before removing its own canary. The backend separately requires the existing strict GetBucketPolicy check. The database and backend-to-proxy connections are plaintext on distinct internal bridges in this candidate. Keep only the named services attached and the VM dedicated. Transport and admin-access policy for those segments remain deployment review gates.

## Guarded first install

This runbook starts after independent exact-source review, a qualified empty VM, fresh disk space evidence, and verified private and off-host recovery targets. SSH, PostgreSQL, storage and Proxmox management must stay private. Do not install on the original Docker Desktop, WSL or Proxmox host.

1. Provision a new dedicated VM only after that target passes its separate preservation review. Install the reviewed guest packages on the new VM. Reserve a new unique 12-character lower-case hex instance ID. Place the reviewed source in a root-owned, clean Git checkout on that VM. Preserve the VM identity and intended disks in a private preflight receipt.
2. Write the single line INSTANCE followed by one space and the VM's /etc/machine-id to /etc/sfd-rorschach-instance, root-owned mode 0600. Create /srv/sfd-rorschach/INSTANCE root-owned mode 0700 outside Git. Put the full reviewed Git SHA in approved-revision there, root-owned mode 0600.
3. Supply secrets.env there, root-owned mode 0600, using the [template](../deploy/rorschach/secrets.env.example). Generate six independent 32-byte lowercase hex secrets with a local cryptographic random source and distinct 20-character uppercase alphanumeric storage identities. Generate the administrator hash locally with a masked bcrypt prompt, cost 12–14. Keep dollars literal in this file. Keep generated credentials in private custody and never print rendered Compose configuration or upload it as evidence.
4. Install fullchain.pem and key.pem in the runtime origin-tls directory. The certificate must cover R1_HOST and the trust policy must match the selected private or public route. Install rustfs_cert.pem and rustfs_key.pem in storage-tls and its signing root as storage-ca.pem. The storage leaf must cover DNS storage. The two keys must belong to UID 10001, mode 0600. Public certificates and CA are root-owned and readable by the services. Keep both mounted TLS directories traversable within their container while restricting the enclosing runtime directory to root. Keep issuer private keys outside the VM runtime directory. The guard checks mode/ownership and rejects symlinks. Strict client handshakes must also verify the actual certificates.
5. Run as the dedicated guest administrator, from the pinned checkout.

   ~~~sh
   python3 -B deploy/rorschach/stack.py INSTANCE validate
   python3 -B deploy/rorschach/stack.py INSTANCE initialize
   python3 -B deploy/rorschach/stack.py INSTANCE status
   ~~~

   Initialize builds the app and provisioner, creates a fresh namespaced project, provisions private storage, then starts the application and loopback proxy. If a phase fails, preserve the project for inspection. A subsequent initialize refuses any matching resource, including same-name unlabeled volumes and networks. Verify local HTTPS with its hostname and CA, then confirm /ready reports PostgreSQL, S3 and native hash healthy. Check the bound sockets and private bridge membership before considering ingress.

6. For routine maintenance, use stop to stop without removing any data. Start requires the previously created full project, revalidates S3 using a fresh canary, then starts the backend and proxy without recreating them. verify-storage is an explicit signed write/read/delete check against a new canary. No command deletes volumes. The existing background database-only backup and file cleanup are disabled in this candidate pending consistent recovery acceptance.

## Remote routing and acceptance

Start with private access to loopback HTTPS. A future Cloudflare route must preserve Host and TLS SNI for the selected name, validate the origin certificate, and keep management traffic off the public route. Use cache bypass for authenticated and download paths, preserve byte identity, do not publish the storage endpoint, and verify trust, header forwarding, real client rate limits and actual request timeouts before enabling the route. If the tunnel collapses all clients to a common peer address, resolve the trusted client-IP design before relying on per-client throttling. Cloudflare documents a 100 MB per-request limit for Free and Pro. The planned upload chunks are at most 32 MiB, and compatibility still needs measured external tests. [Cloudflare HTTP 413 documentation](https://developers.cloudflare.com/support/troubleshooting/http-status-codes/4xx-client-error/error-413/).

Hosted testing uses disposable synthetic TLS identities and new Docker resources. It renders the effective Compose config and exercises signed private S3, anonymous read/head/list/write/delete denial, strict HTTPS, administrator and ordinary-user login, native hash verification, exact-byte signed download and persistence across a cold service stop/start. The suite also requires a signed 65 MiB multipart test and a paired database/object restore to a separate Compose project. Its success does not attest the actual Rorschach VM, off-host recoverability, Internet route or an interactive Windows session.

## Backup, upgrade and rollback gates

Keep the application stopped and verify transfers quiesced before a consistent checkpoint. Collect a PostgreSQL custom-format logical dump and a compatible snapshot of the complete RustFS data directory, including IAM and metadata, with storage stopped. Keep the reviewed source/image manifests, private config, TLS renewal material and ownership metadata as separate recovery dependencies. Record checksums and custody for the pair. Transfer encrypted copies to an independent physical recovery destination. Verify a new isolated restore target has enough free space and no route to the source. Restore matching database, storage and config, then read representative files through the real application and compare SHA-256 and permissions. This is the recovery design. Backup automation and the off-host restore gate are separate from the hosted persistence test.

Before an upgrade, stop intake and capture an independently verified paired checkpoint. Keep the prior reviewed images, configuration and preserved source. Inspect forward migrations before starting a successor. With no new writes, use the known compatible prior image or restore the entire accepted checkpoint to a new isolated target. After new writes, reconcile the database and object deltas before routing traffic back. Never downgrade schema metadata, replace a live storage directory, prune a backup, or copy real Railway data as part of the initial Rorschach release.
